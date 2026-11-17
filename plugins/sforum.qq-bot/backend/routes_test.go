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
	"unicode/utf8"

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
		groupNames: newGroupNameCache(time.Now),
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

// newRouteRequestAs 在 newRouteRequest 上补一个调用者，其余完全一致。
//
// userID<=0 表示「不带 actor」：宿主守卫链路上不会出现这种请求（它要求
// ActorID>0），只有测试需要构造它来钉住兜底行为。
func newRouteRequestAs(t *testing.T, routeID string, body map[string]any, userID int64) *pluginwire.RouteRequest {
	t.Helper()
	request := newRouteRequest(t, routeID, body)
	if userID > 0 {
		request.Context.Actor = &protocolwire.Actor{UserId: userID}
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

	response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
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
		response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
		if err != nil {
			t.Fatalf("第 %d 次建任务失败：%v", attempt+1, err)
		}
		if response.GetStatusCode() != http.StatusBadGateway {
			t.Fatalf("第 %d 次应回报 502，得到 %d", attempt+1, response.GetStatusCode())
		}
	}

	failing.Store(false)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
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
		response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
		if err != nil {
			t.Fatalf("第 %d 次建任务失败：%v", attempt+1, err)
		}
		if response.GetError() != nil {
			t.Fatalf("第 %d 次不该被拒：%+v", attempt+1, response.GetError())
		}
	}
	response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
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

	response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindPollID, nil, testActorUserID))
	if err != nil {
		t.Fatalf("缺参数不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusBadRequest {
		t.Fatalf("状态码期望 400，得到 %d", response.GetStatusCode())
	}
	if detail := response.GetError(); detail == nil || detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("错误码期望 INVALID_ARGUMENT，得到 %+v", detail)
	}

	response, err = instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "missing"}, testActorUserID))
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
	instance.bindStore.Put("task-once", key, testActorUserID)

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

	response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-once"}, testActorUserID))
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

	response, err = instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-once"}, testActorUserID))
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
	instance.bindStore.Put("task-1", "key-1", testActorUserID)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{"retcode": 20001, "msg": "server busy"})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-1"}, testActorUserID))
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
	instance.bindStore.Put("task-race", key, testActorUserID)
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
				newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-race"}, testActorUserID))
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

// bind_start：拿不到调用者就不建任务——凭据必须知道交给谁。
func TestRouteBindStartRejectsMissingActor(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-anon"},
		})
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequestAs(t, routeBindStartID, nil, 0))
	if err != nil {
		t.Fatalf("缺 actor 不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusForbidden {
		t.Fatalf("状态码期望 403，得到 %d", response.GetStatusCode())
	}
	detail := response.GetError()
	if detail == nil || detail.GetCode() != protocolwire.ErrorCode_ERROR_CODE_PERMISSION_DENIED {
		t.Fatalf("错误码期望 PERMISSION_DENIED，得到 %+v", detail)
	}
	if hits.Load() != 0 {
		t.Fatalf("缺 actor 不该打上游，实际 %d 次", hits.Load())
	}
	if instance.bindStore.Len() != 0 {
		t.Fatalf("缺 actor 不该留下任务，实际 %d 条", instance.bindStore.Len())
	}
}

// bind_poll：任务绑定发起者——别人拿到 taskId 也拿不到凭据，发起者仍能交付。
func TestRouteBindPollRejectsOtherOwner(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	var sealed atomic.Value
	sealed.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case createBindTaskPath:
			writeBindJSON(t, writer, map[string]any{
				"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-owner"},
			})
		case pollBindResultPath:
			writeBindJSON(t, writer, map[string]any{
				"retcode": 0, "msg": "", "data": map[string]any{
					"status": bindRemoteCompleted, "bot_appid": "102030405",
					"bot_encrypt_secret": sealed.Load(),
				},
			})
		default:
			t.Errorf("未预期路径：%s", request.URL.Path)
		}
	}))
	defer server.Close()
	instance.bind.baseURL = server.URL

	// 发起者出码：任务表里必须记下发起者，否则下面的校验无从谈起。
	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
	if err != nil {
		t.Fatalf("建任务失败：%v", err)
	}
	routeDocumentValues(t, response, bindStartOutputSchema)
	key, ok := instance.bindStore.Key("task-owner", testActorUserID)
	if !ok {
		t.Fatal("发起者应能读到自己的任务密钥")
	}
	sealed.Store(sealBindSecret(t, key, "owner-only-secret"))

	// 非发起者用同一个 taskId 轮询：只看到 expired，任务原封不动。
	response, err = instance.InvokeRoute(context.Background(),
		newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-owner"}, testOtherUserID))
	if err != nil {
		t.Fatalf("非发起者轮询不该返回 error：%v", err)
	}
	values := routeDocumentValues(t, response, bindPollOutputSchema)
	if values["status"] != bindStatusExpired {
		t.Fatalf("非发起者应回报 expired，得到 %v", values["status"])
	}
	if values["appId"] != "" || values["appSecret"] != "" {
		t.Fatalf("非发起者不该拿到凭据：%v / %v", values["appId"], values["appSecret"])
	}
	if instance.bindStore.Len() != 1 {
		t.Fatalf("非发起者不该把任务取走，实际剩 %d 条", instance.bindStore.Len())
	}

	// 发起者自己轮询：凭据交付，任务被取走。
	response, err = instance.InvokeRoute(context.Background(),
		newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-owner"}, testActorUserID))
	if err != nil {
		t.Fatalf("发起者轮询失败：%v", err)
	}
	values = routeDocumentValues(t, response, bindPollOutputSchema)
	if values["status"] != bindStatusScanned {
		t.Fatalf("发起者应拿到 scanned，得到 %v", values["status"])
	}
	if values["appId"] != "102030405" || values["appSecret"] != "owner-only-secret" {
		t.Fatalf("凭据不对：%v / %v", values["appId"], values["appSecret"])
	}
	if instance.bindStore.Len() != 0 {
		t.Fatalf("凭据交付后任务应被取走，实际还剩 %d 条", instance.bindStore.Len())
	}
}

// bind_poll：请求不带 actor 时回报 expired（200），既不交付凭据也不吃掉任务。
func TestRouteBindPollRejectsMissingActor(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	key, err := bindRandomKey(nil)
	if err != nil {
		t.Fatalf("生成密钥失败：%v", err)
	}
	instance.bindStore.Put("task-anon", key, testActorUserID)

	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequestAs(t, routeBindPollID, map[string]any{"taskId": "task-anon"}, 0))
	if err != nil {
		t.Fatalf("缺 actor 的轮询不该返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("缺 actor 应回报 200 + expired，得到 %d", response.GetStatusCode())
	}
	values := routeDocumentValues(t, response, bindPollOutputSchema)
	if values["status"] != bindStatusExpired {
		t.Fatalf("缺 actor 应回报 expired，得到 %v", values["status"])
	}
	if values["appId"] != "" || values["appSecret"] != "" {
		t.Fatalf("缺 actor 不该拿到凭据：%v / %v", values["appId"], values["appSecret"])
	}
	if _, ok := instance.bindStore.Key("task-anon", testActorUserID); !ok {
		t.Fatal("缺 actor 的轮询不该把任务取走")
	}
}

// preview：只渲染，不发送；截断与未知变量都要报出来。
func TestRoutePreview(t *testing.T) {
	sender := &fakeSender{}
	instance := newRouteTestPlugin(t, testConfig(), sender)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"source":   previewSourceDraft,
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
		"source":          previewSourceDraft,
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
		"source": previewSourceDraft, "template": "摘要：{{excerpt}}", "excerptLimit": 4,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	text, _ := routeDocumentValues(t, response, previewOutputSchema)["text"].(string)
	if !strings.Contains(text, string([]rune(sample)[:4])+"…") {
		t.Fatalf("摘要应按上限截断，得到 %q", text)
	}

	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"source": previewSourceDraft, "template": "摘要：{{excerpt}}", "excerptLimit": 0,
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
		"source": previewSourceDraft, "template": "0123456789abcdefghij", "maxMessageChars": 0,
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

// preview：键在但是空串 → 真的渲染空，不再回落到已保存模板。
func TestRoutePreviewEmptyTemplateRendersEmpty(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequest(t, routePreviewID, map[string]any{"source": previewSourceDraft, "template": ""}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	if text, _ := values["text"].(string); text != "" {
		t.Fatalf("空模板应渲染空串，得到 %q", text)
	}
	unknown, ok := values["unknownVariables"].([]any)
	if !ok || len(unknown) != 0 {
		t.Fatalf("空模板不该报未知变量：%v", values["unknownVariables"])
	}
	if values["truncated"] != false {
		t.Fatalf("空串不该标记 truncated：%v", values["truncated"])
	}
}

// preview：空模板与「maxMessageChars=0 沿用默认上限」两条约定必须共存。
func TestRoutePreviewEmptyTemplateKeepsMaxCharsConvention(t *testing.T) {
	config := testConfig()
	config.MaxMessageChars = 16
	instance := newRouteTestPlugin(t, config, nil)

	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequest(t, routePreviewID, map[string]any{"source": previewSourceDraft, "template": "", "maxMessageChars": 0}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	if text, _ := values["text"].(string); text != "" {
		t.Fatalf("空模板应渲染空串，得到 %q", text)
	}
	if values["truncated"] != false {
		t.Fatalf("空串没有可截断的内容，truncated 应为 false：%v", values["truncated"])
	}
}

// preview：清空签名同样按空串渲染，而不是回落到已保存签名。
func TestRoutePreviewEmptySignatureRendersEmpty(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequest(t, routePreviewID, map[string]any{
			"source": previewSourceDraft, "template": "{{title}}|{{signature}}", "signature": "",
		}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	text, _ := routeDocumentValues(t, response, previewOutputSchema)["text"].(string)
	expected := sampleTemplateData("").Title + "|"
	if text != expected {
		t.Fatalf("空签名应渲染成空，得到 %q（期望 %q）", text, expected)
	}
	if strings.Contains(text, defaultSignature) {
		t.Fatalf("不该回落到已保存签名：%q", text)
	}
}

// preview：source=saved 忽略请求里的全部草稿字段，输出就是已保存配置的渲染。
func TestRoutePreviewSavedSourceIgnoresDraft(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	body := map[string]any{
		"source":          previewSourceSaved,
		"template":        "草稿模板 {{title}}",
		"format":          "markdown",
		"signature":       "草稿签名",
		"excerptLimit":    3,
		"maxMessageChars": 8,
	}
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, body))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	want := "【新主题】" + sampleTemplateData("").Title
	if text, _ := values["text"].(string); text != want {
		t.Fatalf("saved 源应忽略草稿字段，得到 %q（期望 %q）", text, want)
	}
	if values["source"] != previewSourceSaved {
		t.Fatalf("source 应回显 saved：%v", values["source"])
	}
	if values["truncated"] != false {
		t.Fatalf("草稿里的 maxMessageChars=8 不该生效：%v", values["truncated"])
	}
	if characters, ok := values["characters"].(float64); !ok || int(characters) != utf8.RuneCountInString(want) {
		t.Fatalf("characters 应为 %d，得到 %v", utf8.RuneCountInString(want), values["characters"])
	}
	if unknown, _ := values["unknownVariables"].([]any); len(unknown) != 0 {
		t.Fatalf("已保存模板没有未知变量：%v", values["unknownVariables"])
	}

	// 不传 source 时默认就是 saved：同一份 body 去掉 source 必须得到同样的字。
	delete(body, "source")
	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, body))
	if err != nil {
		t.Fatalf("默认源预览失败：%v", err)
	}
	values = routeDocumentValues(t, response, previewOutputSchema)
	if text, _ := values["text"].(string); text != want {
		t.Fatalf("source 缺省应等于 saved，得到 %q", text)
	}
	if values["source"] != previewSourceSaved {
		t.Fatalf("缺省时 source 应回显 saved：%v", values["source"])
	}
}

// preview：source=draft 仍然按请求里的草稿值渲染（缺键回落、未知变量原样保留）。
func TestRoutePreviewDraftSourceStillAppliesDraft(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"source":          previewSourceDraft,
		"template":        "【{{title}}】{{signature}}{{txet}}",
		"signature":       "草稿签名",
		"maxMessageChars": 0,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	want := "【" + sampleTemplateData("").Title + "】草稿签名{{txet}}"
	if text, _ := values["text"].(string); text != want {
		t.Fatalf("draft 源应使用草稿值，得到 %q（期望 %q）", text, want)
	}
	if values["source"] != previewSourceDraft {
		t.Fatalf("source 应回显 draft：%v", values["source"])
	}
	unknown, _ := values["unknownVariables"].([]any)
	if len(unknown) != 1 || unknown[0] != "txet" {
		t.Fatalf("未知变量应原样保留并报出来：%v", values["unknownVariables"])
	}
	if values["truncated"] != false {
		t.Fatalf("maxMessageChars=0 应沿用默认上限，不该截断：%v", values["truncated"])
	}
}

// preview：sample 覆盖只替换给过的字段，摘要仍按生效的 excerptLimit 截断。
func TestRoutePreviewSampleOverride(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "{{title}}|{{author}}|{{excerpt}}"
	instance := newRouteTestPlugin(t, config, nil)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"source": previewSourceSaved,
		"sample": map[string]any{"title": "真实标题", "excerpt": "真实摘要"},
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	want := "真实标题|示例作者|" + BuildExcerpt("真实摘要", config.ExcerptLimit)
	if text, _ := values["text"].(string); text != want {
		t.Fatalf("样例覆盖没生效，得到 %q（期望 %q）", text, want)
	}
	if values["sampleUsed"] != true {
		t.Fatalf("给了 sample 时 sampleUsed 应为 true：%v", values["sampleUsed"])
	}

	// 不给 sample：仍用内置样例，sampleUsed 为 false。
	response, err = instance.InvokeRoute(context.Background(), newRouteRequest(t, routePreviewID, map[string]any{
		"source": previewSourceSaved,
	}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values = routeDocumentValues(t, response, previewOutputSchema)
	if values["sampleUsed"] != false {
		t.Fatalf("没给 sample 时 sampleUsed 应为 false：%v", values["sampleUsed"])
	}
	if text, _ := values["text"].(string); !strings.Contains(text, sampleTemplateData("").Title) {
		t.Fatalf("没给 sample 时应使用内置样例：%q", text)
	}
}

// preview：saved 源与 hook.go 的推送逐字相同——这才是「真预览」的定义。
//
// 做法：用同一份已保存配置跑一次真推送（InvokeHook → dispatcher 队列），再调
// /preview source=saved，比对两边的正文。样例数据的每个字段都由推送载荷提供，
// 所以两条路径喂给 Render 的是同一份 TemplateData。
func TestRoutePreviewSavedSourceMatchesPushPath(t *testing.T) {
	config := testConfig()
	config.MessageTemplate = "【{{title}}】{{author}}/{{category}}\n摘要：{{excerpt}}\n链接：{{url}}\n—— {{signature}}"
	config.ExcerptLimit = 12
	instance, identity := newTestHook(t, config, nil, nil)

	sample := sampleTemplateData(config.Signature)
	payload := map[string]any{
		"topicId": sample.TopicID,
		"topic": map[string]any{
			"id": sample.TopicID, "title": sample.Title, "url": sample.URL, "excerpt": sample.Excerpt,
		},
		"author":   map[string]any{"displayName": sample.Author},
		"category": map[string]any{"name": sample.Category},
	}
	if _, err := instance.InvokeHook(context.Background(),
		newHookRequest(identity, payload, "delivery-preview-parity")); err != nil {
		t.Fatalf("推送路径失败：%v", err)
	}
	var pushed DeliveryJob
	select {
	case pushed = <-instance.dispatcher.queue:
	default:
		t.Fatal("推送路径没有入队任何投递")
	}
	if pushed.Content == "" {
		t.Fatal("推送路径渲染出了空正文")
	}

	response, err := instance.InvokeRoute(context.Background(),
		newRouteRequest(t, routePreviewID, map[string]any{"source": previewSourceSaved}))
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	values := routeDocumentValues(t, response, previewOutputSchema)
	text, _ := values["text"].(string)
	if text != pushed.Content {
		t.Fatalf("saved 预览必须与推送逐字相同：\n预览 %q\n推送 %q", text, pushed.Content)
	}
	if characters, ok := values["characters"].(float64); !ok || int(characters) != utf8.RuneCountInString(pushed.Content) {
		t.Fatalf("characters 与正文不一致：%v vs %q", values["characters"], pushed.Content)
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
			response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, testCase.routeID, testCase.body, testActorUserID))
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

	missingTask, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindPollID, nil, testActorUserID))
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
	upstream, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeBindStartID, nil, testActorUserID))
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
		response, err := instance.InvokeRoute(context.Background(), newRouteRequestAs(t, routeID, map[string]any{"taskId": "x"}, testActorUserID))
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
