package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 与清单 routes[]、schemas/ 逐字一致的契约常量。清单由 pm 维护，这里只读地镜像
// 同一批字符串：任何一侧改动都必须同步。
const (
	routeBindStartID     = "sforum.qq-bot.route.bind_start"
	routeBindPollID      = "sforum.qq-bot.route.bind_poll"
	routePreviewID       = "sforum.qq-bot.route.preview"
	routeTestPushID      = "sforum.qq-bot.route.test_push"
	routeStatusID        = "sforum.qq-bot.route.status"
	routeGroupsListID    = "sforum.qq-bot.route.groups_list"
	routeAccountStatusID = "sforum.qq-bot.route.account_status"

	bindStartOutputSchema     = "sforum.qq-bot.bind.start.output@1"
	bindPollOutputSchema      = "sforum.qq-bot.bind.poll.output@1"
	previewOutputSchema       = "sforum.qq-bot.preview.output@1"
	testPushOutputSchema      = "sforum.qq-bot.test_push.output@1"
	statusOutputSchema        = "sforum.qq-bot.status.output@1"
	groupsListOutputSchema    = "sforum.qq-bot.groups.list.output@1"
	accountStatusOutputSchema = "sforum.qq-bot.account.status.output@1"

	bindStartInputSchema     = "sforum.qq-bot.bind.start.input@1"
	bindPollInputSchema      = "sforum.qq-bot.bind.poll.input@1"
	previewInputSchema       = "sforum.qq-bot.preview.input@1"
	testPushInputSchema      = "sforum.qq-bot.test_push.input@1"
	statusInputSchema        = "sforum.qq-bot.status.input@1"
	groupsListInputSchema    = "sforum.qq-bot.groups.list.input@1"
	accountStatusInputSchema = "sforum.qq-bot.account.status.input@1"
)

// 路由侧的可读拒绝文案。它们只出现在 ErrorDetail 或 {ok:false, message} 里，
// 绝不包含任何凭据。
const (
	messageBindUnavailable      = "扫码绑定服务尚未就绪，请重启插件后再试，或直接手动填写凭据。"
	messageBindRateLimited      = "扫码绑定请求过于频繁，请稍后再试。"
	messageBindTaskMissing      = "缺少 taskId，请先调用 /bind/start 生成二维码。"
	messageBindActorMissing     = "无法确认调用者身份，请重新登录后再试。"
	messagePreviewSourceInvalid = "source 只接受 saved 或 draft。"
)

// 预览的两个数据源。
//
//	saved（默认）：用**已保存配置**渲染，和 hook.go 的推送共用 renderSaved——
//	  这就是「真正会发出去的那段字」。
//	draft：用请求里的草稿值渲染，供运营者编辑时实时看效果。
const (
	previewSourceSaved = "saved"
	previewSourceDraft = "draft"
)

// routeContentTypeJSON 是路由响应的媒体类型。
//
// 宿主按 RouteResponse.Headers 原样写进 HTTP 响应，插件不声明就落到 fiber 的默认
// text/plain；浏览器随即把 JSON body 当字符串返回，前端 $fetch 拿到的是 string 而
// 不是 {text}，组件只能当成失败。所有出口（成功与错误）都必须带上它。
const routeContentTypeJSON = "application/json; charset=utf-8"

// InvokeRoute 覆盖嵌入的 pluginv2.Server 默认实现（默认返回 Unimplemented）。
// 六条扩展路由都在这里分发：组件模式下宿主不渲染 actions[]，测试推送、状态、
// 模板预览与群列表只能走插件自己的路由。
func (p *plugin) InvokeRoute(ctx context.Context, request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	switch request.GetRouteId() {
	case routeBindStartID:
		return p.routeBindStart(ctx, request)
	case routeBindPollID:
		return p.routeBindPoll(ctx, request)
	case routePreviewID:
		return p.routePreview(request)
	case routeTestPushID:
		return p.routeTestPush(ctx, request)
	case routeStatusID:
		return p.routeStatus(request)
	case routeGroupsListID:
		return p.routeGroupsList(ctx, request)
	case routeAccountStatusID:
		return p.routeAccountStatus(ctx, request)
	default:
		return routeFailure(request, http.StatusNotFound,
			protocolwire.ErrorCode_ERROR_CODE_NOT_FOUND, "qq_bot.route_unknown",
			"未声明的 QQ 机器人路由"), nil
	}
}

// routeBindStart 申请一个扫码绑定任务，返回二维码内容与有效期。
func (p *plugin) routeBindStart(ctx context.Context, request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	if p.bind == nil || p.bindStore == nil {
		return routeFailure(request, http.StatusServiceUnavailable,
			protocolwire.ErrorCode_ERROR_CODE_UNAVAILABLE, "qq_bot.bind_unavailable", messageBindUnavailable), nil
	}
	// 没有发起者就不建任务：凭据必须知道交给谁。放在限流之前，免得一次没有
	// 归属的请求还吃掉运营者的配额。
	ownerID := routeActorID(request)
	if ownerID <= 0 {
		return routeFailure(request, http.StatusForbidden,
			protocolwire.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "qq_bot.bind_actor_missing", messageBindActorMissing), nil
	}
	if wait := p.bindStore.AllowStart(); wait > 0 {
		seconds := int(math.Ceil(wait.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		return routeFailure(request, http.StatusTooManyRequests,
			protocolwire.ErrorCode_ERROR_CODE_RATE_LIMITED, "qq_bot.bind_rate_limited",
			fmt.Sprintf("%s（约 %d 秒后可再试）", messageBindRateLimited, seconds)), nil
	}
	task, err := p.bind.CreateBindTask(ctx)
	if err != nil {
		// 上游失败不消耗配额：限流挡的是「建任务」这个动作，不是网络抖动。
		p.bindStore.RefundStart()
		p.log.Warnf("qq-bot: 新建扫码绑定任务失败：%s", bindAdminMessage(err))
		return routeFailure(request, http.StatusBadGateway,
			protocolwire.ErrorCode_ERROR_CODE_UNAVAILABLE, "qq_bot.bind_start_failed", bindAdminMessage(err)), nil
	}
	p.bindStore.Put(task.TaskID, task.Key, ownerID)
	return p.routeDocument(request, bindStartOutputSchema, map[string]any{
		"taskId":           task.TaskID,
		"connectUrl":       task.ConnectURL,
		"expiresInSeconds": int(p.bindStore.TTL() / time.Second),
	})
}

// routeBindPoll 查询扫码进度。轮询可以反复读，但凭据只交付一次：一旦拿到
// appSecret 立即从任务表取走，重复轮询只会看到 expired。
func (p *plugin) routeBindPoll(ctx context.Context, request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	if p.bind == nil || p.bindStore == nil {
		return routeFailure(request, http.StatusServiceUnavailable,
			protocolwire.ErrorCode_ERROR_CODE_UNAVAILABLE, "qq_bot.bind_unavailable", messageBindUnavailable), nil
	}
	taskID := routeStringValue(routeBody(request), "taskId")
	if taskID == "" {
		return routeFailure(request, http.StatusBadRequest,
			protocolwire.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "qq_bot.bind_task_missing", messageBindTaskMissing), nil
	}
	// 非发起者与「任务不存在」走同一条出口：都回报 expired、都不带凭据。
	// 选 expired 而不是一个「非本人」错误码，理由有两条：前端已处理 expired
	// 语义（bind-card 零改动），且它不告诉非本人「这个 taskId 真的存在」。
	// 缺 actor 时 ownerID 为 0，与任何真实任务都不匹配，同样落到 expired。
	ownerID := routeActorID(request)
	key, found := p.bindStore.Key(taskID, ownerID)
	if !found {
		return p.routeDocument(request, bindPollOutputSchema, bindPollDocument(bindStatusExpired, "", ""))
	}
	result, err := p.bind.PollBindTask(ctx, taskID, key)
	if err != nil {
		p.log.Warnf("qq-bot: 轮询扫码绑定结果失败：%s", bindAdminMessage(err))
		return routeFailure(request, http.StatusBadGateway,
			protocolwire.ErrorCode_ERROR_CODE_UNAVAILABLE, "qq_bot.bind_poll_failed", bindAdminMessage(err)), nil
	}
	if result.Status != bindStatusScanned {
		if result.Status == bindStatusExpired {
			p.bindStore.Delete(taskID)
		}
		return p.routeDocument(request, bindPollOutputSchema,
			bindPollDocument(result.Status, "", ""))
	}
	// 成功只此一次：以 Take 的结果为准，而不是本地的轮询结果。两个并发轮询都拿到
	// 同一份凭据时，只有抢到任务的那个能交付，另一个回报 expired。
	if _, owned := p.bindStore.Take(taskID, ownerID); !owned {
		return p.routeDocument(request, bindPollOutputSchema, bindPollDocument(bindStatusExpired, "", ""))
	}
	return p.routeDocument(request, bindPollOutputSchema,
		bindPollDocument(bindStatusScanned, result.AppID, result.AppSecret))
}

// routePreview 只渲染，不发送、不查 HostQuery：预览 runtime 拿不到 host token，
// 也不该为了一次预览去读论坛正文。
//
// source=saved（默认）忽略请求里的全部草稿字段：运营者要的是「保存之后真正会发
// 出去的那段字」，草稿预览会让他以为没保存也生效。source=draft 保留编辑期预览。
func (p *plugin) routePreview(request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	values := routeBody(request)
	source := routeStringValue(values, "source")
	if source == "" {
		source = previewSourceSaved
	}
	if source != previewSourceSaved && source != previewSourceDraft {
		return routeFailure(request, http.StatusBadRequest,
			protocolwire.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "qq_bot.preview_source_invalid",
			messagePreviewSourceInvalid), nil
	}

	draft := resolvePreviewDraft(p.cfg, values)
	if source == previewSourceSaved {
		// 已保存模式不读任何草稿字段：模板、形态、签名、摘要上限与总长度上限
		// 全部取配置，连「0 = 沿用默认」这条约定都不必解释。
		draft = previewDraftFromConfig(p.cfg)
	}
	sample := routeSampleOverride(values)
	data := sample.apply(sampleTemplateDataFor(draft.Signature, draft.ExcerptLimit), draft.ExcerptLimit)
	rendered := p.renderPreviewSource(source, draft, data)

	// structpb 只认 []any；空切片也要给，否则 JSON 里是 null 而不是 []。
	unknown := make([]any, 0, len(rendered.Unknown))
	for _, name := range rendered.Unknown {
		unknown = append(unknown, name)
	}
	payload := map[string]any{
		"source":           source,
		"text":             rendered.Text,
		"truncated":        rendered.Truncated,
		"characters":       utf8.RuneCountInString(rendered.Text),
		"unknownVariables": unknown,
		"sampleUsed":       sample.used(),
	}
	if source == previewSourceSaved {
		// savedSnapshot 是**服务端真相**：本次渲染实际用的已保存配置。宿主给组件
		// 的 settings.items 是挂载时的值快照（bridge 全文件只调一次），保存成功后
		// 组件拿不到新值，而 save() 失败也不 reject——组件只能靠这份回显来分辨
		// 「真的保存了」与「保存失败」。draft 源刻意不带，避免被当成基线。
		payload["savedSnapshot"] = savedSnapshotDocument(p.cfg)
	}
	return p.routeDocument(request, previewOutputSchema, payload)
}

// savedSnapshotDocument 是已保存配置的只读投影，键名与清单设置项逐字一致，
// 让设置页组件可以直接把它当成「保存后的基线」。
func savedSnapshotDocument(config Config) map[string]any {
	return map[string]any{
		"message_template":  config.MessageTemplate,
		"message_format":    config.MessageFormat,
		"signature":         config.Signature,
		"excerpt_limit":     config.ExcerptLimit,
		"max_message_chars": config.MaxMessageChars,
	}
}

// renderPreviewSource 按数据源选渲染入口：saved 与推送共用 renderSaved，draft 用
// 请求里的草稿值。两个分支的差别只有「值从哪来」，渲染实现是同一个 Render。
func (p *plugin) renderPreviewSource(source string, draft previewDraft, data TemplateData) Rendered {
	if source == previewSourceSaved {
		return p.renderSaved(data)
	}
	return Render(draft.Template, data, RenderOptions{
		Format: draft.Format, MaxChars: draft.MaxMessageChars, Warnf: p.log.Warnf,
	})
}

// renderSaved 是「真正会发出去的那段字」的唯一渲染入口：模板、消息形态、总长度
// 上限与 Warnf 全部来自已保存配置。hook.go 的推送与 source=saved 的预览都只走
// 这里——任何一侧偷偷换了参数，TestRoutePreviewSavedSourceMatchesPushPath 就会红。
func (p *plugin) renderSaved(data TemplateData) Rendered {
	return Render(p.cfg.MessageTemplate, data, RenderOptions{
		Format: p.cfg.MessageFormat, MaxChars: p.cfg.MaxMessageChars, Warnf: p.log.Warnf,
	})
}

// renderPreview 是草稿预览的渲染入口：路由（source=draft）与设置页工具共用，
// 保证同一份草稿在两处得到逐字相同的结果。
func (p *plugin) renderPreview(draft previewDraft) Rendered {
	return Render(draft.Template, sampleTemplateDataFor(draft.Signature, draft.ExcerptLimit), RenderOptions{
		Format: draft.Format, MaxChars: draft.MaxMessageChars, Warnf: p.log.Warnf,
	})
}

// routeTestPush 用**已保存配置**发一条样例到白名单第一个群。群为空或凭据缺失
// 都返回 ok:false 与可读原因——这不是异常，是管理员还没配完。
func (p *plugin) routeTestPush(ctx context.Context, request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	config := p.cfg
	rendered := Render(config.MessageTemplate, sampleTemplateDataFor(config.Signature, config.ExcerptLimit), RenderOptions{
		Format: config.MessageFormat, MaxChars: config.MaxMessageChars, Warnf: p.log.Warnf,
	})
	var report strings.Builder
	report.WriteString(rendered.Text)
	report.WriteString("\n\n—— 发送结果 ——\n")

	ok := false
	sender := p.messageSender()
	switch {
	case len(config.GroupIDs) == 0:
		report.WriteString("未发送：群白名单是空的，请先填写至少一个群 ID。")
	case !config.CredentialsConfigured():
		report.WriteString("未发送：尚未配置 AppID / AppSecret。")
	case sender == nil:
		report.WriteString("未发送：QQ 客户端未初始化。")
	default:
		target := config.GroupIDs[0]
		result, err := sender.SendGroupMessage(ctx, target, rendered.Text, config.MessageFormat)
		if err != nil {
			fmt.Fprintf(&report, "发送失败：%s", adminReason(err))
		} else {
			ok = true
			fmt.Fprintf(&report, "已发送到群 %s（message id %s…）", maskGroupID(target), messageIDPrefix(result.MessageID))
		}
	}
	return p.routeDocument(request, testPushOutputSchema, map[string]any{
		"ok": ok, "message": report.String(),
	})
}

// routeStatus 复用设置页工具的状态文本，保持两条入口看到同一份事实。
func (p *plugin) routeStatus(request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	return p.routeDocument(request, statusOutputSchema, map[string]any{"text": p.renderStatus()})
}

// routeGroupsList 返回插件自己收到的群事件里识别到的群。
//
// QQ 群侧没有「列出机器人所在群」的接口（只有频道有 /users/@me/guilds），所以
// 唯一数据源是 ClaimRecord：机器人必须先在群里收到过事件，那个群才会出现在这里。
// 群名是尽力而为的附加信息，拿不到就返回 null + nameUnavailable，绝不让整个
// 列表因为名字查询失败而失败。
func (p *plugin) routeGroupsList(ctx context.Context, request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	var claims []ClaimRecord
	if p.state != nil {
		claims = p.state.RecentClaims(claimRingSize)
	}
	openIDs := make([]string, 0, len(claims))
	for _, claim := range claims {
		openIDs = append(openIDs, claim.GroupID)
	}
	names := p.resolveGroupNames(ctx, openIDs)

	// structpb 只认 []any；空切片也要给，否则 JSON 里是 null 而不是 []。
	groups := make([]any, 0, len(claims))
	for _, claim := range claims {
		name := names[claim.GroupID]
		var nameValue any
		if name != "" {
			nameValue = name
		}
		groups = append(groups, map[string]any{
			"openid":          claim.GroupID,
			"name":            nameValue,
			"nameUnavailable": name == "",
			"eventType":       claim.EventType,
			"firstSeenAt":     claim.FirstSeen.UTC().Format(time.RFC3339),
			"lastSeenAt":      claim.LastSeen.UTC().Format(time.RFC3339),
			"count":           claim.Count,
		})
	}
	return p.routeDocument(request, groupsListOutputSchema, map[string]any{
		"groups":         groups,
		"claimEnabled":   p.cfg.EnableGroupIDClaim,
		"recordingLimit": claimRingSize,
	})
}

// routeAccountStatus 是设置页「账号状态」卡片的唯一数据源：机器人身份（带缓存）、
// 网关状态（只读进程内状态，不发请求）、群数量与检查时间。
//
// 它永远返回 200：机器人信息拿不到只是 available=false + 可读 reason，卡片其余
// 部分照常显示——运营者打开设置页不该因为平台抖动而看到一整块错误。
func (p *plugin) routeAccountStatus(ctx context.Context, request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	now := time.Now()
	profile, reason := p.accountBot(ctx)
	var botReason any
	if reason != "" {
		botReason = reason
	}
	recognized := 0
	if p.state != nil {
		recognized = len(p.state.RecentClaims(claimRingSize))
	}
	return p.routeDocument(request, accountStatusOutputSchema, map[string]any{
		"appId":                 p.cfg.AppID,
		"credentialsConfigured": p.cfg.CredentialsConfigured(),
		"bot": map[string]any{
			"name":      profile.Name,
			"avatarUrl": profile.AvatarURL,
			"available": reason == "",
			"reason":    botReason,
		},
		"gateway": accountGateway(p.gateway.Status(), now),
		"groups": map[string]any{
			"recognized":  recognized,
			"whitelisted": countUniqueGroups(p.cfg.GroupIDs),
		},
		"checkedAt": now.UTC().Format(time.RFC3339),
	})
}

// messageSender 是投递出口：生产是 *QQClient，测试注入假实现。
func (p *plugin) messageSender() Sender {
	if p == nil {
		return nil
	}
	if p.sender != nil {
		return p.sender
	}
	if p.client != nil {
		return p.client
	}
	return nil
}

// previewDraft 是预览的生效值：请求里**缺**哪一项才回落到已保存配置。
//
// Template 与 Signature 是 presence-aware 的：键在就按给的值渲染，空串就渲染空
// （清空模板必须看到空预览，而不是旧模板的渲染结果）。MaxMessageChars 是例外，
// 清单约定 0 表示沿用插件默认值，不是「不限长」。
type previewDraft struct {
	Template        string
	Format          string
	Signature       string
	ExcerptLimit    int
	MaxMessageChars int
}

// previewDraftFromConfig 把已保存配置摊成草稿形态。saved 预览与推送共用同一份值：
// 模板、形态、签名、摘要上限、总长度上限一个不落，来源只有 config。
func previewDraftFromConfig(config Config) previewDraft {
	return previewDraft{
		Template:        config.MessageTemplate,
		Format:          config.MessageFormat,
		Signature:       config.Signature,
		ExcerptLimit:    config.ExcerptLimit,
		MaxMessageChars: config.MaxMessageChars,
	}
}

func resolvePreviewDraft(config Config, values map[string]any) previewDraft {
	draft := previewDraftFromConfig(config)
	if template, ok := routeStringPresent(values, "template"); ok {
		draft.Template = template
	}
	if format := routeStringValue(values, "format"); format != "" {
		draft.Format = normalizeFormat(format)
	}
	if signature, ok := routeStringPresent(values, "signature"); ok {
		draft.Signature = signature
	}
	if limit, ok := routeIntValue(values, "excerptLimit"); ok {
		draft.ExcerptLimit = limit
	}
	// 清单 preview.input 约定：0 表示沿用插件默认值，而不是「不限长」。
	if limit, ok := routeIntValue(values, "maxMessageChars"); ok && limit > 0 {
		draft.MaxMessageChars = limit
	}
	return draft
}

// previewSample 是请求里给的样例覆盖。字段为 nil 表示没给，仍用内置样例——
// 只有真正给过的字段才覆盖，这样「只换标题验排版」不会顺手清掉作者与摘要。
type previewSample struct {
	title    *string
	author   *string
	category *string
	excerpt  *string
	url      *string
}

func (s previewSample) used() bool {
	return s.title != nil || s.author != nil || s.category != nil || s.excerpt != nil || s.url != nil
}

// apply 把覆盖叠到样例数据上。摘要仍按本次生效的 excerptLimit 截断：样例换了，
// 「摘要长度上限」这条语义不能跟着换。
func (s previewSample) apply(data TemplateData, excerptLimit int) TemplateData {
	if s.title != nil {
		data.Title = *s.title
	}
	if s.author != nil {
		data.Author = *s.author
	}
	if s.category != nil {
		data.Category = *s.category
	}
	if s.url != nil {
		data.URL = *s.url
	}
	if s.excerpt != nil {
		data.Excerpt = BuildExcerpt(*s.excerpt, excerptLimit)
	}
	return data
}

// routeSampleOverride 读请求里的 sample 对象。缺对象、字段不是字符串都当作没给。
func routeSampleOverride(values map[string]any) previewSample {
	raw, ok := values["sample"]
	if !ok || raw == nil {
		return previewSample{}
	}
	sample, ok := raw.(map[string]any)
	if !ok {
		return previewSample{}
	}
	return previewSample{
		title:    routeSampleField(sample, "title"),
		author:   routeSampleField(sample, "author"),
		category: routeSampleField(sample, "category"),
		excerpt:  routeSampleField(sample, "excerpt"),
		url:      routeSampleField(sample, "url"),
	}
}

func routeSampleField(sample map[string]any, key string) *string {
	text, ok := routeStringPresent(sample, key)
	if !ok {
		return nil
	}
	return &text
}

// sampleTemplateDataFor 在样例数据上叠加草稿的摘要长度：把「摘要长度上限」调小，
// 预览里能立刻看到截断，而不是等到真实推送。
func sampleTemplateDataFor(signature string, excerptLimit int) TemplateData {
	data := sampleTemplateData(signature)
	data.Excerpt = BuildExcerpt(data.Excerpt, excerptLimit)
	return data
}

func bindPollDocument(status, appID, appSecret string) map[string]any {
	return map[string]any{"status": status, "appId": appID, "appSecret": appSecret}
}

// routeJSONResponse 是路由响应的唯一出口：成功与错误都从这里出去，因此
// Content-Type 不可能被漏掉。新增路由请只走 routeDocument / routeFailure。
func routeJSONResponse(
	request *pluginwire.RouteRequest,
	status int,
	document *protocolwire.TypedDocument,
	detail *protocolwire.ErrorDetail,
) *pluginwire.RouteResponse {
	return &pluginwire.RouteResponse{
		Context:    routeResponseContext(request.GetContext()),
		StatusCode: uint32(status),
		// 单值 Content-Type：宿主逐个 Add 进 HTTP 响应头。
		Headers: []*protocolwire.Header{{
			Name:   "Content-Type",
			Values: []string{routeContentTypeJSON},
		}},
		Body:  document,
		Error: detail,
	}
}

// routeDocument 把值绑到声明的 schema 上。schema ref 必须与清单逐字一致，否则
// 宿主按 exact-ref 校验会直接拒绝这份响应。
func (p *plugin) routeDocument(request *pluginwire.RouteRequest, schemaRef string, values map[string]any) (*pluginwire.RouteResponse, error) {
	document, err := pluginv2.NewTypedDocument(schemaRef, values)
	if err != nil {
		return nil, err
	}
	return routeJSONResponse(request, http.StatusOK, document, nil), nil
}

// routeFailure 拼一条可读拒绝。Code 决定宿主侧 HTTP 映射，Message 直接给管理员看；
// 响应体为空，但 Content-Type 依然声明 JSON，客户端可以无条件按 JSON 解析。
func routeFailure(request *pluginwire.RouteRequest, status int, code protocolwire.ErrorCode, reason, message string) *pluginwire.RouteResponse {
	return routeJSONResponse(request, status, nil, &protocolwire.ErrorDetail{
		Code: code, Reason: reason, Message: message,
	})
}

// routeResponseContext 回填与请求完全一致的 context：宿主会逐字段比对 request_id、
// trace 与 extension 身份，并拒绝缺失 server_time 的响应。
func routeResponseContext(request *protocolwire.RequestContext) *protocolwire.ResponseContext {
	response := &protocolwire.ResponseContext{ServerTime: timestamppb.New(time.Now().UTC())}
	if request == nil {
		return response
	}
	response.RequestId = request.GetRequestId()
	if trace := request.GetTrace(); trace != nil {
		response.Trace = proto.Clone(trace).(*protocolwire.TraceContext)
	}
	if extension := request.GetExtension(); extension != nil {
		response.Extension = proto.Clone(extension).(*protocolwire.ExtensionIdentity)
	}
	return response
}

// routeBody 读请求体的 map 形态；没有 body 时返回 nil，取值函数对 nil 宽容。
func routeBody(request *pluginwire.RouteRequest) map[string]any {
	if request.GetBody() == nil {
		return nil
	}
	return pluginv2.TypedDocumentValues(request.GetBody())
}

func routeStringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	if text, ok := raw.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

// routeStringPresent 读一个字符串字段，并额外报告「这个键到底给没给」。
//
// 预览要区分「缺键」（回落已保存配置）与「键在但是空串」（按空串渲染），
// routeStringValue 把两者都变成 ""，所以需要这个版本。null、非字符串一律按
// 「没给」处理：schema 里这些字段就是 string。
func routeStringPresent(values map[string]any, key string) (string, bool) {
	if values == nil {
		return "", false
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return "", false
	}
	text, isString := raw.(string)
	if !isString {
		return "", false
	}
	return strings.TrimSpace(text), true
}

// routeActorID 取调用者账号，取不到返回 0。
//
// 五条路由都挂在 core.guard.permission 后面，宿主已要求 ActorID>0 才放行
// （并在派发时把 actor 送进插件），所以这里再取一次不是复读权限判定，而是给
// 「将来新增无守卫入口」留一道兜底：没有账号就不该产生任何有归属的副作用。
func routeActorID(request *pluginwire.RouteRequest) int64 {
	if request == nil {
		return 0
	}
	return request.GetContext().GetActor().GetUserId()
}

// routeIntValue 读一个非负整数。structpb 把数字统一成 float64，但也接受字符串，
// 免得前端换一种序列化就整段回落到默认值。
func routeIntValue(values map[string]any, key string) (int, bool) {
	if values == nil {
		return 0, false
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch typed := raw.(type) {
	case float64:
		if typed < 0 || math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return int(typed), true
	case string:
		value, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || value < 0 {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}
