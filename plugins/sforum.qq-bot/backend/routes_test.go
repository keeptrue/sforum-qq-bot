package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// newRouteTestPlugin 组装一个具备路由依赖的插件实例：假投递出口、真任务表、
// 指向 httptest 的扫码客户端。
func newRouteTestPlugin(t *testing.T, config Config, sender Sender) *plugin {
	t.Helper()
	if sender == nil {
		sender = &fakeSender{}
	}
	log := testLogger()
	state := NewState()
	instance := &plugin{
		cfg:        config,
		log:        log,
		state:      state,
		client:     newQQClient(config, log),
		dispatcher: NewDispatcher(config, sender, state, log),
		gateway:    newGateway(config, nil, state, log),
		sender:     sender,
		bind:       newBindClient(log),
		bindStore:  NewBindStore(time.Now),
	}
	instance.Server = pluginv2.NewServer()
	return instance
}

func newRouteRequest(t *testing.T, routeID string, body map[string]any) *pluginwire.RouteRequest {
	t.Helper()
	request := &pluginwire.RouteRequest{
		Context:         &protocolwire.RequestContext{RequestId: "req-route", Extension: testIdentity()},
		RouteId:         routeID,
		ContractVersion: routeID + "@1",
	}
	if body != nil {
		document, err := pluginv2.NewTypedDocument("sforum.qq-bot.test.input@1", body)
		if err != nil {
			t.Fatalf("构造请求体失败：%v", err)
		}
		request.Body = document
	}
	return request
}

func routeDocumentValues(t *testing.T, response *pluginwire.RouteResponse, schemaRef string) map[string]any {
	t.Helper()
	if response == nil {
		t.Fatal("路由返回了 nil 响应")
	}
	if detail := response.GetError(); detail != nil {
		t.Fatalf("路由返回错误：%s / %s", detail.GetReason(), detail.GetMessage())
	}
	requireJSONContentType(t, response)
	if !pluginv2.DocumentMatchesSchema(response.GetBody(), schemaRef) {
		t.Fatalf("响应 schema 不是 %s：%+v", schemaRef, response.GetBody())
	}
	return pluginv2.TypedDocumentValues(response.GetBody())
}

// routeHeaderValue 取响应头（大小写不敏感），多个值用逗号拼起来。
func routeHeaderValue(response *pluginwire.RouteResponse, name string) string {
	for _, header := range response.GetHeaders() {
		if strings.EqualFold(header.GetName(), name) {
			return strings.Join(header.GetValues(), ",")
		}
	}
	return ""
}

// requireJSONContentType 断言响应声明了 JSON。宿主按 RouteResponse.Headers 原样
// 写回 HTTP；缺了它浏览器会把 JSON 当字符串，前端拿到 string 而不是对象。
func requireJSONContentType(t *testing.T, response *pluginwire.RouteResponse) {
	t.Helper()
	if response == nil {
		t.Fatal("路由返回了 nil 响应")
	}
	if count := len(response.GetHeaders()); count != 1 {
		t.Fatalf("应只声明一个响应头（Content-Type），实际 %d 个：%+v", count, response.GetHeaders())
	}
	contentType := routeHeaderValue(response, "Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") {
		t.Fatalf("Content-Type 必须是 JSON，实际 %q", contentType)
	}
	if contentType != routeContentTypeJSON {
		t.Fatalf("Content-Type 与约定不一致：%q", contentType)
	}
}

// 未知路由必须给 ERROR_CODE_NOT_FOUND。
func TestInvokeRouteUnknown(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, "sforum.qq-bot.route.nope", nil))
	if err != nil {
		t.Fatalf("未知路由不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusNotFound {
		t.Fatalf("状态码期望 404，得到 %d", response.GetStatusCode())
	}
	detail := response.GetError()
	if detail == nil || detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_NOT_FOUND {
		t.Fatalf("错误码期望 NOT_FOUND，得到 %+v", detail)
	}
}

// 响应 context 必须与请求逐字段一致，并带合法 server_time：宿主会拒绝不匹配的响应。
func TestInvokeRouteResponseContext(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	request := newRouteRequest(t, routeStatusID, nil)
	response, err := instance.InvokeRoute(context.Background(), request)
	if err != nil {
		t.Fatalf("查询状态失败：%v", err)
	}
	context := response.GetContext()
	if context.GetRequestId() != request.GetContext().GetRequestId() {
		t.Fatalf("request_id 不一致：%q", context.GetRequestId())
	}
	if context.GetExtension().GetExtensionId() != extensionID {
		t.Fatalf("extension 身份缺失：%+v", context.GetExtension())
	}
	if context.GetServerTime() == nil || !context.GetServerTime().IsValid() {
		t.Fatalf("server_time 非法：%+v", context.GetServerTime())
	}
}

// bind_start：建任务 → 返回 taskId / connectUrl / expiresInSeconds，并落进任务表。
func TestRouteBindStart(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != createBindTaskPath {
			t.Errorf("未预期路径：%s", request.URL.Path)
		}
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-route"},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL
	instance.bind.connectBaseURL = "https://q.qq.com"

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindStartID, nil))
	if err != nil {
		t.Fatalf("建任务失败：%v", err)
	}
	values := routeDocumentValues(t, response, bindStartOutputSchema)
	if values["taskId"] != "task-route" {
		t.Fatalf("taskId 不对：%v", values["taskId"])
	}
	connectURL, _ := values["connectUrl"].(string)
	if connectURL != "https://q.qq.com/qqbot/openclaw/connect.html?task_id=task-route&source=sforum&_wv=2" {
		t.Fatalf("connectUrl 不符合协议：%s", connectURL)
	}
	expires, ok := values["expiresInSeconds"].(float64)
	if !ok || int(expires) != int(bindTaskTTL/time.Second) {
		t.Fatalf("expiresInSeconds 期望 %d，得到 %v", int(bindTaskTTL/time.Second), values["expiresInSeconds"])
	}
	if instance.bindStore.Len() != 1 {
		t.Fatalf("任务表应有 1 条任务，实际 %d 条", instance.bindStore.Len())
	}
}

// bind_start：上游失败不消耗配额——连失败 5 次之后仍应能成功建任务。
func TestRouteBindStartRefundsQuotaOnUpstreamFailure(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	var failing atomic.Bool
	failing.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			writeBindJSON(t, writer, map[string]any{"retcode": 20001, "msg": "server busy"})
			return
		}
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-ok"},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	for attempt := 0; attempt < bindStartPerMinute; attempt++ {
		response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindStartID, nil))
		if err != nil {
			t.Fatalf("第 %d 次建任务失败：%v", attempt+1, err)
		}
		if response.GetStatusCode() != http.StatusBadGateway {
			t.Fatalf("第 %d 次应回报 502，得到 %d", attempt+1, response.GetStatusCode())
		}
	}

	failing.Store(false)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindStartID, nil))
	if err != nil {
		t.Fatalf("回滚配额后建任务失败：%v", err)
	}
	values := routeDocumentValues(t, response, bindStartOutputSchema)
	if values["taskId"] != "task-ok" {
		t.Fatalf("上次失败不该吃掉配额，实际被限流：%v", values)
	}
}

// bind_start 限流：每分钟 5 次，第 6 次 429 + RATE_LIMITED。
func TestRouteBindStartRateLimited(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hits++
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-" + string(rune('a'+hits))},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	for attempt := 0; attempt < bindStartPerMinute; attempt++ {
		response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindStartID, nil))
		if err != nil {
			t.Fatalf("第 %d 次建任务失败：%v", attempt+1, err)
		}
		if response.GetError() != nil {
			t.Fatalf("第 %d 次不该被拒：%+v", attempt+1, response.GetError())
		}
	}
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindStartID, nil))
	if err != nil {
		t.Fatalf("限流不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusTooManyRequests {
		t.Fatalf("状态码期望 429，得到 %d", response.GetStatusCode())
	}
	if detail := response.GetError(); detail == nil || detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_RATE_LIMITED {
		t.Fatalf("错误码期望 RATE_LIMITED，得到 %+v", detail)
	}
	if hits != bindStartPerMinute {
		t.Fatalf("被限流后不该再打上游，实际请求 %d 次", hits)
	}
}

// bind_poll：缺 taskId → 400；任务不存在 → expired。
func TestRouteBindPollRejections(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindPollID, nil))
	if err != nil {
		t.Fatalf("缺参数不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusBadRequest {
		t.Fatalf("状态码期望 400，得到 %d", response.GetStatusCode())
	}
	if detail := response.GetError(); detail == nil || detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("错误码期望 INVALID_ARGUMENT，得到 %+v", detail)
	}

	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindPollID, map[string]any{"taskId": "missing"}))
	if err != nil {
		t.Fatalf("未知任务不该返回 error：%v", err)
	}
	values := routeDocumentValues(t, response, bindPollOutputSchema)
	if values["status"] != bindStatusExpired {
		t.Fatalf("未知任务应回报 expired，得到 %v", values["status"])
	}
	if values["appId"] != "" || values["appSecret"] != "" {
		t.Fatalf("过期任务不该带凭据：%v / %v", values["appId"], values["appSecret"])
	}
}

// bind_poll：扫码成功后凭据只交付一次，第二次轮询回落 expired。
func TestRouteBindPollDeliversSecretOnce(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	key, err := bindRandomKey(nil)
	if err != nil {
		t.Fatalf("生成密钥失败：%v", err)
	}
	instance.bindStore.Put("task-once", key)

	var sealed string
	sealed = sealBindSecret(t, key, "app-secret-once")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			TaskID string `json:"task_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("轮询请求体不是 JSON：%v", err)
		}
		if body.TaskID != "task-once" {
			t.Errorf("任务号不对：%q", body.TaskID)
		}
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{
				"status": bindRemoteCompleted, "bot_appid": "102030405", "bot_encrypt_secret": sealed,
			},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindPollID, map[string]any{"taskId": "task-once"}))
	if err != nil {
		t.Fatalf("轮询失败：%v", err)
	}
	values := routeDocumentValues(t, response, bindPollOutputSchema)
	if values["status"] != bindStatusScanned {
		t.Fatalf("状态期望 scanned，得到 %v", values["status"])
	}
	if values["appId"] != "102030405" || values["appSecret"] != "app-secret-once" {
		t.Fatalf("凭据不对：%v / %v", values["appId"], values["appSecret"])
	}
	if instance.bindStore.Len() != 0 {
		t.Fatalf("凭据交付后任务应被取走，实际还剩 %d 条", instance.bindStore.Len())
	}

	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindPollID, map[string]any{"taskId": "task-once"}))
	if err != nil {
		t.Fatalf("二次轮询失败：%v", err)
	}
	values = routeDocumentValues(t, response, bindPollOutputSchema)
	if values["status"] != bindStatusExpired {
		t.Fatalf("二次轮询应回报 expired，得到 %v", values["status"])
	}
}

// bind_poll：上游失败要落到可读原因，而不是空响应。
func TestRouteBindPollUpstreamFailure(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	instance.bindStore.Put("task-1", "key-1")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{"retcode": 20001, "msg": "server busy"})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindPollID, map[string]any{"taskId": "task-1"}))
	if err != nil {
		t.Fatalf("上游失败不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusBadGateway {
		t.Fatalf("状态码期望 502，得到 %d", response.GetStatusCode())
	}
	detail := response.GetError()
	if detail == nil || detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_UNAVAILABLE {
		t.Fatalf("错误码期望 UNAVAILABLE，得到 %+v", detail)
	}
	if !strings.Contains(detail.GetMessage(), "retcode 20001") {
		t.Fatalf("原因里应带 retcode：%q", detail.GetMessage())
	}
}

// bind_poll：并发轮询同一个已完成任务时，凭据只能交付一次。
func TestRouteBindPollConcurrentDeliveryIsExclusive(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	key, err := bindRandomKey(nil)
	if err != nil {
		t.Fatalf("生成密钥失败：%v", err)
	}
	sealed := sealBindSecret(t, key, "only-once")
	instance.bindStore.Put("task-race", key)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{
				"status": bindRemoteCompleted, "bot_appid": "102030405", "bot_encrypt_secret": sealed,
			},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	const pollers = 8
	type outcome struct {
		status string
		secret string
	}
	results := make(chan outcome, pollers)
	var waitGroup sync.WaitGroup
	for index := 0; index < pollers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			response, invokeErr := instance.InvokeRoute(context.Background(),
				newRouteRequest(t, routeBindPollID, map[string]any{"taskId": "task-race"}))
			if invokeErr != nil {
				results <- outcome{status: "error:" + invokeErr.Error()}
				return
			}
			values := pluginv2.TypedDocumentValues(response.GetBody())
			status, _ := values["status"].(string)
			secret, _ := values["appSecret"].(string)
			results <- outcome{status: status, secret: secret}
		}()
	}
	waitGroup.Wait()
	close(results)

	delivered := 0
	for result := range results {
		if result.status == bindStatusScanned {
			delivered++
			if result.secret != "only-once" {
				t.Fatalf("交付的凭据不对：%q", result.secret)
			}
		}
	}
	if delivered != 1 {
		t.Fatalf("凭据应只交付一次，实际交付 %d 次", delivered)
	}
	if instance.bindStore.Len() != 0 {
		t.Fatalf("任务应已被取走，实际还剩 %d 条", instance.bindStore.Len())
	}
}

// preview：只渲染，不发送；截断与未知变量都要报出来。
func TestRoutePreview(t *testing.T) {
	sender := &fakeSender{}
	instance := newRouteTestPlugin(t, testConfig(), sender)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"template": "[{{title}}]{{txet}}",
		"format":   "text",
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	text, _ := values["text"].(string)
	if !strings.Contains(text, "{{txet}}") {
		t.Fatalf("未知变量应原样保留：%q", text)
	}
	unknown, ok := values["unknownVariables"].([]any)
	if !ok || len(unknown) != 1 || unknown[0] != "txet" {
		t.Fatalf("未知变量清单不对：%v", values["unknownVariables"])
	}
	if values["truncated"] != false {
		t.Fatalf("未超长不该标记 truncated：%v", values["truncated"])
	}

	// 单独一段：把总长度压到 8 字，必须报 truncated。
	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"template":        "0123456789abcdef",
		"maxMessageChars": 8,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values = routeDocumentValues(t, response, previewOutputSchema)
	text, _ = values["text"].(string)
	if values["truncated"] != true {
		t.Fatalf("超长文本应标记 truncated：%v", values["truncated"])
	}
	if text != "01234567…" {
		t.Fatalf("截断结果不对：%q", text)
	}
	if calls := sender.Calls(); len(calls) != 0 {
		t.Fatalf("预览不该发送任何消息，实际 %d 次", len(calls))
	}
}

// preview：请求里缺项时回落到已保存配置，而不是渲染空模板。
func TestRoutePreviewFallsBackToSavedConfig(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, nil))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	text, _ := values["text"].(string)
	if !strings.Contains(text, "【新主题】") {
		t.Fatalf("应回落已保存模板，得到 %q", text)
	}
	if values["truncated"] != false {
		t.Fatalf("样例不该被截断：%v", values["truncated"])
	}
}

// preview：摘要长度上限会对样例摘要生效，0 表示不显示摘要。
func TestRoutePreviewExcerptLimit(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	sample := sampleTemplateData("sig").Excerpt

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"template": "摘要：{{excerpt}}", "excerptLimit": 4,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	text, _ := routeDocumentValues(t, response, previewOutputSchema)["text"].(string)
	if !strings.Contains(text, string([]rune(sample)[:4])+"…") {
		t.Fatalf("摘要应按上限截断，得到 %q", text)
	}

	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"template": "摘要：{{excerpt}}", "excerptLimit": 0,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	text, _ = routeDocumentValues(t, response, previewOutputSchema)["text"].(string)
	if text != "摘要：" {
		t.Fatalf("摘要上限为 0 时应为空，得到 %q", text)
	}
}

// preview：maxMessageChars=0 按清单约定回落到插件默认值，而不是「不限长」。
func TestRoutePreviewMaxCharsZeroUsesDefault(t *testing.T) {
	config := testConfig()
	config.MaxMessageChars = 16
	instance := newRouteTestPlugin(t, config, nil)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"template": "0123456789abcdefghij", "maxMessageChars": 0,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	if values["truncated"] != true {
		t.Fatalf("0 应回落到 16 字上限并截断：%v", values["text"])
	}
	if text, _ := values["text"].(string); text != "0123456789abcdef…" {
		t.Fatalf("截断结果不对：%q", text)
	}
}

// test_push：空群、无凭据、正常三条路径。
func TestRouteTestPushRejections(t *testing.T) {
	sender := &fakeSender{}
	instance := newRouteTestPlugin(t, testConfig(), sender)
	instance.cfg.GroupIDs = nil

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeTestPushID, nil))
	if err != nil {
		t.Fatalf("测试推送失败：%v", err)
	}
	values := routeDocumentValues(t, response, testPushOutputSchema)
	if values["ok"] != false {
		t.Fatalf("空群应返回 ok:false：%v", values["ok"])
	}
	if message, _ := values["message"].(string); !strings.Contains(message, "群白名单是空的") {
		t.Fatalf("空群原因不对：%q", message)
	}

	withoutCredentials := newRouteTestPlugin(t, testConfig(), sender)
	withoutCredentials.cfg.AppSecret = ""
	response, err = withoutCredentials.InvokeRoute(context.Background(), newRouteRequest(t, routeTestPushID, nil))
	if err != nil {
		t.Fatalf("测试推送失败：%v", err)
	}
	values = routeDocumentValues(t, response, testPushOutputSchema)
	if values["ok"] != false {
		t.Fatalf("缺凭据应返回 ok:false：%v", values["ok"])
	}
	if message, _ := values["message"].(string); !strings.Contains(message, "AppSecret") {
		t.Fatalf("缺凭据原因不对：%q", message)
	}
	if calls := sender.Calls(); len(calls) != 0 {
		t.Fatalf("被拒绝时不该发送，实际 %d 次", len(calls))
	}
}

func TestRouteTestPushSends(t *testing.T) {
	sender := &fakeSender{}
	instance := newRouteTestPlugin(t, testConfig(), sender)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeTestPushID, nil))
	if err != nil {
		t.Fatalf("测试推送失败：%v", err)
	}
	values := routeDocumentValues(t, response, testPushOutputSchema)
	if values["ok"] != true {
		message, _ := values["message"].(string)
		t.Fatalf("应发送成功，得到 ok=%v message=%q", values["ok"], message)
	}
	if message, _ := values["message"].(string); !strings.Contains(message, "已发送到群") {
		t.Fatalf("成功文案不对：%q", message)
	}
	calls := sender.Calls()
	if len(calls) != 1 || calls[0].group != "GROUP_A" {
		t.Fatalf("应只发到白名单第一个群，实际 %+v", calls)
	}
	if calls[0].format != messageFormatText {
		t.Fatalf("消息形态应沿用已保存配置，得到 %q", calls[0].format)
	}
}

func TestRouteTestPushSendFailure(t *testing.T) {
	sender := &fakeSender{behavior: func(int, string) (SendResult, error) {
		return SendResult{}, &QQError{Reason: "凭据无效：AppID 或 AppSecret 不正确"}
	}}
	instance := newRouteTestPlugin(t, testConfig(), sender)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeTestPushID, nil))
	if err != nil {
		t.Fatalf("测试推送失败：%v", err)
	}
	values := routeDocumentValues(t, response, testPushOutputSchema)
	if values["ok"] != false {
		t.Fatalf("发送失败应返回 ok:false：%v", values["ok"])
	}
	if message, _ := values["message"].(string); !strings.Contains(message, "发送失败：凭据无效") {
		t.Fatalf("失败原因应可读：%q", message)
	}
}

// status：直接复用 renderStatus 的文本。
func TestRouteStatus(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeStatusID, nil))
	if err != nil {
		t.Fatalf("查询状态失败：%v", err)
	}
	values := routeDocumentValues(t, response, statusOutputSchema)
	text, _ := values["text"].(string)
	if !strings.Contains(text, "QQ 推送状态") {
		t.Fatalf("状态文本不对：%q", text)
	}
	if strings.Contains(text, testConfig().AppSecret) {
		t.Fatal("状态文本泄露了 AppSecret")
	}
}

// 五条路由的成功响应都必须声明 JSON，且 body 绑定到各自 schema。
func TestRouteResponsesAreJSON(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), &fakeSender{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-json"},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	cases := []struct {
		name      string
		routeID   string
		body      map[string]any
		schemaRef string
	}{
		{"bind_start", routeBindStartID, nil, bindStartOutputSchema},
		{"bind_poll", routeBindPollID, map[string]any{"taskId": "不存在的任务"}, bindPollOutputSchema},
		{"preview", routePreviewID, map[string]any{"template": "{{title}}"}, previewOutputSchema},
		{"test_push", routeTestPushID, nil, testPushOutputSchema},
		{"status", routeStatusID, nil, statusOutputSchema},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, testCase.routeID, testCase.body))
			if err != nil {
				t.Fatalf("调用 %s 失败：%v", testCase.name, err)
			}
			if response.GetStatusCode() != http.StatusOK {
				t.Fatalf("%s 状态码期望 200，得到 %d", testCase.name, response.GetStatusCode())
			}
			requireJSONContentType(t, response)
			if !pluginv2.DocumentMatchesSchema(response.GetBody(), testCase.schemaRef) {
				t.Fatalf("%s 的响应 schema 不是 %s：%+v", testCase.name, testCase.schemaRef, response.GetBody())
			}
		})
	}
}

// 错误响应同样要声明 JSON：客户端不该为 4xx/5xx 换一套解析逻辑。
func TestRouteErrorResponsesAreJSON(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)

	unknown, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, "sforum.qq-bot.route.nope", nil))
	if err != nil {
		t.Fatalf("未知路由不该返回 error：%v", err)
	}
	requireJSONContentType(t, unknown)
	if unknown.GetStatusCode() != http.StatusNotFound {
		t.Fatalf("未知路由状态码期望 404，得到 %d", unknown.GetStatusCode())
	}

	missingTask, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindPollID, nil))
	if err != nil {
		t.Fatalf("缺参数不该返回 error：%v", err)
	}
	requireJSONContentType(t, missingTask)
	if missingTask.GetStatusCode() != http.StatusBadRequest {
		t.Fatalf("缺 taskId 状态码期望 400，得到 %d", missingTask.GetStatusCode())
	}

	failingServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{"retcode": 20001, "msg": "server busy"})
	}))
	defer failingServer.Close()
	instance.bind.baseURL = failingServer.URL
	upstream, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeBindStartID, nil))
	if err != nil {
		t.Fatalf("上游失败不该返回 error：%v", err)
	}
	requireJSONContentType(t, upstream)
	if upstream.GetStatusCode() != http.StatusBadGateway {
		t.Fatalf("上游失败状态码期望 502，得到 %d", upstream.GetStatusCode())
	}
}

// 扫码绑定组件未就绪时给可读拒绝，而不是 panic 或空响应。
func TestRouteBindUnavailable(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	instance.bind = nil
	instance.bindStore = nil

	for _, routeID := range []string{routeBindStartID, routeBindPollID} {
		response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeID, map[string]any{"taskId": "x"}))
		if err != nil {
			t.Fatalf("%s 不该返回 error：%v", routeID, err)
		}
		if response.GetStatusCode() != http.StatusServiceUnavailable {
			t.Fatalf("%s 状态码期望 503，得到 %d", routeID, response.GetStatusCode())
		}
		if detail := response.GetError(); detail == nil || detail.GetMessage() == "" {
			t.Fatalf("%s 应带可读原因，得到 %+v", routeID, detail)
		}
	}
}
