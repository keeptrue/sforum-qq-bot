package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

type queryStub struct {
	mu    sync.Mutex
	calls []string
	text  string
	err   error
	delay time.Duration
}

func (q *queryStub) query(ctx context.Context, _ *protocolwire.RequestContext, topicID string) (string, error) {
	q.mu.Lock()
	q.calls = append(q.calls, topicID)
	text, err, delay := q.text, q.err, q.delay
	q.mu.Unlock()
	if delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
	}
	return text, err
}

func (q *queryStub) Calls() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.calls...)
}

func resultValues(t *testing.T, response *pluginwire.HookResponse) (bool, int) {
	t.Helper()
	if detail := response.GetError(); detail != nil {
		t.Fatalf("响应带错误：%s（%s）", detail.GetReason(), detail.GetMessage())
	}
	if !response.GetAccepted() {
		t.Fatal("响应应为 Accepted")
	}
	values := pluginv2.TypedDocumentValues(response.GetResult())
	accepted, _ := values["accepted"].(bool)
	queued, _ := values["queuedGroups"].(float64)
	if !accepted {
		t.Fatalf("Result.accepted 应为 true：%v", values)
	}
	return accepted, int(queued)
}

func TestInvokeHookRendersAndEnqueues(t *testing.T) {
	config := testConfig()
	config.GroupIDs = []string{"GROUP_A", "GROUP_B"}
	config.MessageTemplate = "【新主题】{{title}}\n作者：{{author}}\n分类：{{category}}\n标签：{{tags}}\n摘要：{{excerpt}}\n链接：{{url}}"
	config.ExcerptLimit = 10
	query := &queryStub{text: "这是主题正文的开头部分，足够被截断"}
	instance, identity := newTestHook(t, config, nil, query.query)

	start := time.Now()
	response, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1"))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if elapsed > hookBudget {
		t.Fatalf("handler 必须在 5 秒内返回，实际 %v", elapsed)
	}
	_, queued := resultValues(t, response)
	if queued != 2 {
		t.Fatalf("两个白名单群应入队 2 个目标，得到 %d", queued)
	}
	if calls := query.Calls(); len(calls) != 1 || calls[0] != "12345" {
		t.Fatalf("摘要查询应命中主题 12345，得到 %v", calls)
	}
	if pending := instance.dispatcher.Pending(); pending != 1 {
		t.Fatalf("队列里应有一条投递任务，实际 %d", pending)
	}
	job := <-instance.dispatcher.queue
	if len(job.Groups) != 2 || job.Groups[0] != "GROUP_A" {
		t.Fatalf("任务目标群不对：%v", job.Groups)
	}
	for _, want := range []string{"【新主题】新版本发布", "作者：老后", "分类：公告", "标签：发布 · 版本",
		"摘要：这是主题正文的开头部…", "链接：https://forum.example.com/t/12345"} {
		if !strings.Contains(job.Content, want) {
			t.Fatalf("渲染内容缺少 %q：\n%s", want, job.Content)
		}
	}
	if job.TopicTitle != "新版本发布" {
		t.Fatalf("任务标题不对：%q", job.TopicTitle)
	}
}

func TestInvokeHookSkipsWithoutError(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"暂停", func(c *Config) { c.Paused = true }},
		{"缺凭据", func(c *Config) { c.AppID = "" }},
		{"白名单为空", func(c *Config) { c.GroupIDs = nil }},
		{"分类不匹配", func(c *Config) { c.CategoryFilter = []string{"news"} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			config := testConfig()
			testCase.mutate(&config)
			query := &queryStub{text: "不该被查"}
			instance, identity := newTestHook(t, config, nil, query.query)

			response, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1"))
			if err != nil {
				t.Fatalf("过滤不应返回 Go 错误：%v", err)
			}
			_, queued := resultValues(t, response)
			if queued != 0 {
				t.Fatalf("过滤后 queuedGroups 应为 0，得到 %d", queued)
			}
			if len(query.Calls()) != 0 {
				t.Fatalf("过滤后不应查询摘要：%v", query.Calls())
			}
			if pending := instance.dispatcher.Pending(); pending != 0 {
				t.Fatalf("过滤后不应入队，实际 %d", pending)
			}
		})
	}
}

func TestInvokeHookRejectsUnknownHook(t *testing.T) {
	instance, identity := newTestHook(t, testConfig(), nil, nil)
	request := newHookRequest(identity, fullTopicPayload(), "delivery-1")
	request.HookId = extensionID + ".observe.other"

	response, err := instance.InvokeHook(context.Background(), request)
	if err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if response.GetAccepted() {
		t.Fatal("未知 hook 不应被接受")
	}
	detail := response.GetError()
	if detail == nil {
		t.Fatal("未知 hook 应带错误详情")
	}
	if detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_NOT_FOUND {
		t.Fatalf("错误码应为 NOT_FOUND，得到 %v", detail.GetCode())
	}
	if detail.GetReason() != "qq_bot.hook_not_found" {
		t.Fatalf("错误原因不对：%q", detail.GetReason())
	}
}

// 回归：HookId 必须是事件 id。旧实现把清单里的 handler 当 HookId 比对，
// 结果每条 topic.created 都被判成未知 hook（推送完全不触发）。
func TestInvokeHookRejectsHandlerIDAsHookID(t *testing.T) {
	instance, identity := newTestHook(t, testConfig(), nil, nil)
	request := newHookRequest(identity, fullTopicPayload(), "delivery-1")
	request.HookId = observeHandlerID

	response, err := instance.InvokeHook(context.Background(), request)
	if err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if response.GetAccepted() {
		t.Fatal("handler id 不是 HookId，必须被拒")
	}
	if detail := response.GetError(); detail == nil ||
		detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_NOT_FOUND {
		t.Fatalf("应返回 NOT_FOUND：%v", detail)
	}
}

func TestInvokeHookToleratesQueryFailure(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "{{title}}\n{{excerpt}}"
	query := &queryStub{err: errors.New("query_not_allowed")}
	instance, identity := newTestHook(t, config, nil, query.query)

	response, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1"))
	if err != nil {
		t.Fatalf("查询失败不应变成 Go 错误：%v", err)
	}
	_, queued := resultValues(t, response)
	if queued != 1 {
		t.Fatalf("查询失败仍应尝试推送，得到 %d", queued)
	}
	job := <-instance.dispatcher.queue
	if strings.TrimSpace(job.Content) != "新版本发布" {
		t.Fatalf("摘要应为空且不留多余内容：%q", job.Content)
	}
}

func TestInvokeHookSkipsQueryWhenTemplateDoesNotUseExcerpt(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "{{title}}"
	query := &queryStub{text: "不该被查"}
	instance, identity := newTestHook(t, config, nil, query.query)

	if _, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1")); err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if len(query.Calls()) != 0 {
		t.Fatalf("模板不含摘要变量时不应查询：%v", query.Calls())
	}
}

func TestInvokeHookExcerptLimitZeroSkipsQuery(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "{{title}}|{{excerpt}}"
	config.ExcerptLimit = 0
	query := &queryStub{text: "不该被查"}
	instance, identity := newTestHook(t, config, nil, query.query)

	if _, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1")); err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if len(query.Calls()) != 0 {
		t.Fatalf("excerpt_limit=0 时不应查询：%v", query.Calls())
	}
	job := <-instance.dispatcher.queue
	if job.Content != "新版本发布|" {
		t.Fatalf("摘要应为空串：%q", job.Content)
	}
}

func TestInvokeHookDeduplicatesDelivery(t *testing.T) {
	instance, identity := newTestHook(t, testConfig(), nil, nil)
	payload := fullTopicPayload()
	if _, err := instance.InvokeHook(context.Background(), newHookRequest(identity, payload, "delivery-1")); err != nil {
		t.Fatalf("首次投递失败：%v", err)
	}
	response, err := instance.InvokeHook(context.Background(), newHookRequest(identity, payload, "delivery-1"))
	if err != nil {
		t.Fatalf("重复投递不应返回 Go 错误：%v", err)
	}
	_, queued := resultValues(t, response)
	if queued != 0 {
		t.Fatalf("重复投递不应入队，得到 %d", queued)
	}
	if pending := instance.dispatcher.Pending(); pending != 1 {
		t.Fatalf("队列里应只有一条，实际 %d", pending)
	}
}

func TestInvokeHookPropagatesTruncation(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "{{title}}"
	config.MaxMessageChars = 3
	instance, identity := newTestHook(t, config, nil, nil)

	if _, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1")); err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	job := <-instance.dispatcher.queue
	if !job.Truncated {
		t.Fatal("超长消息应把 truncated 传给投递任务")
	}
	if job.Content != "新版本…" {
		t.Fatalf("截断结果不对：%q", job.Content)
	}
}

func TestInvokeHookQueryRespectsBudget(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "{{title}}|{{excerpt}}"
	query := &queryStub{text: "很慢的正文", delay: 5 * time.Second}
	instance, identity := newTestHook(t, config, nil, query.query)

	start := time.Now()
	response, err := instance.InvokeHook(context.Background(), newHookRequest(identity, fullTopicPayload(), "delivery-1"))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if elapsed > hookBudget {
		t.Fatalf("摘要查询必须被 2 秒子预算掐断，整体耗时 %v", elapsed)
	}
	if elapsed < topicQueryBudget/2 {
		t.Fatalf("查询应真的被等待到子预算，耗时 %v", elapsed)
	}
	_, queued := resultValues(t, response)
	if queued != 1 {
		t.Fatalf("超时后仍应推送，得到 %d", queued)
	}
	job := <-instance.dispatcher.queue
	if job.Content != "新版本发布|" {
		t.Fatalf("超时后摘要应为空：%q", job.Content)
	}
}

func TestInvokeHookRejectsPayloadWithoutTopicID(t *testing.T) {
	instance, identity := newTestHook(t, testConfig(), nil, nil)
	payload := map[string]any{"topic": map[string]any{"title": "没有 id"}}

	response, err := instance.InvokeHook(context.Background(), newHookRequest(identity, payload, "delivery-1"))
	if err != nil {
		t.Fatalf("InvokeHook 返回错误：%v", err)
	}
	if response.GetAccepted() {
		t.Fatal("缺少主题 id 不应被接受")
	}
	if detail := response.GetError(); detail == nil ||
		detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("应返回 INVALID_ARGUMENT：%v", detail)
	}
}

func TestDecodeTopicEventFallsBackToFlatFields(t *testing.T) {
	event := decodeTopicEvent(map[string]any{
		"topicId":      "777",
		"title":        "扁平标题",
		"authorName":   "扁平作者",
		"categorySlug": "tech",
		"categoryName": "技术",
		"url":          "https://forum.example.com/t/777",
		"tags":         []any{"标签一", "标签二"},
	})
	if event.TopicID != "777" || event.Title != "扁平标题" || event.AuthorName != "扁平作者" {
		t.Fatalf("扁平字段解码不对：%+v", event)
	}
	if event.CategorySlug != "tech" || event.CategoryName != "技术" {
		t.Fatalf("分类解码不对：%+v", event)
	}
	if len(event.Tags) != 2 || event.Tags[0] != "标签一" {
		t.Fatalf("标签解码不对：%+v", event.Tags)
	}
}

func TestDecodeTopicEventAcceptsNumericID(t *testing.T) {
	event := decodeTopicEvent(map[string]any{"topic": map[string]any{"id": float64(10086)}})
	if event.TopicID != "10086" {
		t.Fatalf("数字 id 应转成十进制字符串，得到 %q", event.TopicID)
	}
}
