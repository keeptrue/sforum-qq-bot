package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	hostwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

const (
	// hookBudget 是 observe 观察者对自己设的硬上限：宿主的事件投递不应该被 QQ
	// 接口拖住，所以除摘要查询外的全部工作都是内存操作。
	hookBudget = 5 * time.Second
	// topicQueryBudget 是摘要查询的子预算。
	topicQueryBudget = 2 * time.Second
)

// topicEvent 是 topic.created 载荷里本插件用到的部分。宿主的事件载荷同时存在
// 扁平字段与嵌套对象两种写法，解码时两种都读，嵌套优先。
type topicEvent struct {
	TopicID      string
	Title        string
	Slug         string
	URL          string
	AuthorName   string
	CategorySlug string
	CategoryName string
	Tags         []string
	Excerpt      string
}

// InvokeHook 是 observe 观察者的唯一入口。
func (p *plugin) InvokeHook(ctx context.Context, request *pluginwire.HookRequest) (*pluginwire.HookResponse, error) {
	response := &pluginwire.HookResponse{
		Context: &protocolwire.ResponseContext{
			RequestId: request.GetContext().GetRequestId(),
			Extension: request.GetContext().GetExtension(),
		},
		Accepted: false,
	}
	// HookId 是**事件 id**，不是清单里的 handler：宿主按 declaration.ID 填充
	// （protocol_v2_client.go 的 `HookId: declaration.ID`）。比 handler 会让每条
	// topic.created 都被拒，且失败只出现在宿主日志里。
	if request.GetHookId() != observeEventID || request.GetHookName() != observeEventName {
		return hookFailure(response, protocolwire.ErrorCode_ERROR_CODE_NOT_FOUND,
			"qq_bot.hook_not_found", "This plugin only observes topic.created."), nil
	}
	// 运行时身份由握手绑定：Health 不通过说明制品已过期，先拒绝再说。
	health, err := p.Server.Health(ctx, &protocolwire.HealthRequest{Context: request.GetContext()})
	if err != nil {
		return nil, err
	}
	if health.GetError() != nil {
		response.Error = health.GetError()
		return response, nil
	}

	// 整体预算：无论摘要查询多慢，handler 都必须在 5 秒内返回。
	budget, cancel := context.WithTimeout(ctx, hookBudget)
	defer cancel()

	event := decodeTopicEvent(pluginv2.TypedDocumentValues(request.GetPayload()))
	if event.TopicID == "" {
		return hookFailure(response, protocolwire.ErrorCode_ERROR_CODE_INVALID_ARGUMENT,
			"qq_bot.topic_required", "topic.created must carry a topic id."), nil
	}

	// 过滤顺序固定：暂停 → 凭据缺失 → 白名单为空 → 分类不匹配 → 去重。
	// 命中任何一条都不是失败：事件被观察者看过并正常接受，只是没有投递目标。
	admitted, skipReason := p.dispatcher.Admit(request.GetDeliveryId(), event.TopicID, event.CategorySlug)
	if !admitted {
		p.log.Infof("qq-bot: 跳过 topic.created（%s）", skipReason)
		return acceptedHookResponse(response, 0)
	}

	excerpt := event.Excerpt
	if p.cfg.ExcerptLimit > 0 && TemplateMentions(p.cfg.MessageTemplate, templateVarExcerpt, "摘要") {
		if strings.TrimSpace(excerpt) == "" {
			// 摘要查询必须在本次 InvokeHook 的同步路径内完成：request.GetContext()
			// 属于 RPC 作用域，handler 返回后再用它会失效。
			excerpt = p.loadExcerpt(budget, request, event.TopicID)
		}
	}
	excerpt = BuildExcerpt(excerpt, p.cfg.ExcerptLimit)

	rendered := Render(p.cfg.MessageTemplate, TemplateData{
		Title:     event.Title,
		Author:    event.AuthorName,
		Category:  event.CategoryName,
		Tags:      event.Tags,
		Excerpt:   excerpt,
		URL:       event.URL,
		TopicID:   event.TopicID,
		Signature: p.cfg.Signature,
	}, RenderOptions{
		Format: p.cfg.MessageFormat, MaxChars: p.cfg.MaxMessageChars, Warnf: p.log.Warnf,
	})
	if strings.TrimSpace(rendered.Text) == "" {
		p.log.Warnf("qq-bot: 渲染结果为空，跳过本次投递（模板可能只含未知变量）")
		return acceptedHookResponse(response, 0)
	}
	if rendered.Truncated {
		p.log.Infof("qq-bot: 渲染结果超长，已按 %d 字截断", p.cfg.MaxMessageChars)
	}

	groups := append([]string(nil), p.cfg.GroupIDs...)
	p.dispatcher.Enqueue(DeliveryJob{
		TopicTitle: event.Title,
		Content:    rendered.Text,
		Groups:     groups,
		Truncated:  rendered.Truncated,
	})
	return acceptedHookResponse(response, len(groups))
}

// loadExcerpt 同步取主题正文摘要。查询失败不阻断推送：摘要留空、记一条 warn。
func (p *plugin) loadExcerpt(budget context.Context, request *pluginwire.HookRequest, topicID string) string {
	queryCtx, cancel := context.WithTimeout(budget, topicQueryBudget)
	defer cancel()
	plain, err := p.plainTextFor(queryCtx, request.GetContext(), topicID)
	if err != nil {
		p.log.Warnf("qq-bot: 主题摘要查询失败，按空摘要继续：%s", sanitizeReason(err))
		return ""
	}
	return plain
}

// plainTextFor 走可替换的查询入口，生产路径是 HostQuery，测试注入假实现。
func (p *plugin) plainTextFor(ctx context.Context, parent *protocolwire.RequestContext, topicID string) (string, error) {
	if p.topicQuery != nil {
		return p.topicQuery(ctx, parent, topicID)
	}
	return p.queryTopicPlainText(ctx, parent, topicID)
}

// queryTopicPlainText 通过稳定 HostQuery 合同取主题正文。filter 字段 id 必须是
// 十进制字符串，值文档走宿主唯一的通用过滤 schema。
func (p *plugin) queryTopicPlainText(ctx context.Context, parent *protocolwire.RequestContext, topicID string) (string, error) {
	host, err := p.Host()
	if err != nil {
		return "", errors.New("host broker unavailable")
	}
	filterValue, err := pluginv2.NewHostQueryFilterValue(topicID)
	if err != nil {
		return "", err
	}
	response, err := host.Queries.Execute(ctx, &hostwire.QueryRequest{
		Context:     host.RequestContext(parent),
		QueryId:     topicQueryID,
		PlanVersion: topicQueryPlanVersion,
		Fields:      []string{"id", "plain_text"},
		Filters:     []*hostwire.QueryFilter{{Field: "id", Operator: "eq", Value: filterValue}},
	})
	if err != nil {
		return "", err
	}
	if response.GetError() != nil {
		return "", errors.New(response.GetError().GetReason())
	}
	if len(response.GetRows()) == 0 {
		return "", nil
	}
	return stringValue(pluginv2.TypedDocumentValues(response.GetRows()[0]), "plain_text"), nil
}

func acceptedHookResponse(response *pluginwire.HookResponse, queuedGroups int) (*pluginwire.HookResponse, error) {
	document, err := pluginv2.NewTypedDocument(observeResultSchemaRef, map[string]any{
		"accepted": true, "queuedGroups": float64(queuedGroups),
	})
	if err != nil {
		return hookFailure(response, protocolwire.ErrorCode_ERROR_CODE_INTERNAL,
			"qq_bot.result_schema_mismatch", err.Error()), nil
	}
	response.Accepted = true
	response.Result = document
	return response, nil
}

func hookFailure(response *pluginwire.HookResponse, code protocolwire.ErrorCode, reason, detail string) *pluginwire.HookResponse {
	response.Accepted = false
	response.Error = &protocolwire.ErrorDetail{Code: code, Reason: reason, Message: detail}
	return response
}

// decodeTopicEvent 兼容嵌套对象与扁平字段两种载荷形态。
func decodeTopicEvent(values map[string]any) topicEvent {
	topic := nestedValues(values, "topic")
	author := nestedValues(values, "author")
	category := nestedValues(values, "category")

	event := topicEvent{
		TopicID:      firstNonEmpty(stringValue(topic, "id"), stringValue(values, "topicId", "topic_id", "id")),
		Title:        firstNonEmpty(stringValue(topic, "title"), stringValue(values, "title")),
		Slug:         firstNonEmpty(stringValue(topic, "slug"), stringValue(values, "slug")),
		URL:          firstNonEmpty(stringValue(topic, "url"), stringValue(values, "url", "permalink")),
		AuthorName:   firstNonEmpty(stringValue(author, "displayName", "display_name"), stringValue(author, "username")),
		CategorySlug: firstNonEmpty(stringValue(category, "slug"), stringValue(values, "categorySlug", "category_slug")),
		CategoryName: firstNonEmpty(stringValue(category, "name"), stringValue(values, "categoryName", "category_name")),
		Excerpt:      firstNonEmpty(stringValue(topic, "excerpt", "plain_text", "plainText"), stringValue(values, "excerpt")),
		Tags:         tagNames(values, topic),
	}
	if event.AuthorName == "" {
		event.AuthorName = stringValue(values, "authorName", "author_name", "username")
	}
	return event
}

func nestedValues(values map[string]any, key string) map[string]any {
	nested, ok := values[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return nested
}

// stringValue 依次尝试若干字段名，返回第一个非空的字符串形态值。数字（例如 id）
// 按无小数部分格式化，避免 12345 变成 "1.2345e+04"。
func stringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		case bool:
			if typed {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

// tagNames 读取 tags[]；元素既可能是对象（取 name），也可能是裸字符串。
func tagNames(values map[string]any, topic map[string]any) []string {
	raw, ok := values["tags"]
	if !ok {
		raw = topic["tags"]
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		switch typed := item.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				names = append(names, trimmed)
			}
		case map[string]any:
			if name := stringValue(typed, "name", "title", "slug"); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// sanitizeReason 把内部错误压成一行、限长，且不携带请求体或凭证。
func sanitizeReason(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if text == "" {
		return "unknown"
	}
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return text
}
