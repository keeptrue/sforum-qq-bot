package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// accountStatusHarness 只模拟 GET /users/@me（外加取凭证）。
type accountStatusHarness struct {
	server *httptest.Server

	mu      sync.Mutex
	meCalls int
	status  int
	payload map[string]any
}

func newAccountStatusHarness(t *testing.T, status int, payload map[string]any) *accountStatusHarness {
	t.Helper()
	harness := &accountStatusHarness{status: status, payload: payload}
	harness.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case tokenPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-1", "expires_in": 7200})
		case "/users/@me":
			harness.mu.Lock()
			harness.meCalls++
			status, payload := harness.status, harness.payload
			harness.mu.Unlock()
			writeJSON(t, writer, status, payload)
		default:
			t.Errorf("意外路径：%s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	}))
	t.Cleanup(harness.server.Close)
	return harness
}

func (h *accountStatusHarness) calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.meCalls
}

// newAccountStatusPlugin 组装一个指向假 API 的路由测试插件。
func newAccountStatusPlugin(t *testing.T, config Config, harness *accountStatusHarness) *plugin {
	t.Helper()
	instance := newRouteTestPlugin(t, config, nil)
	instance.client.baseURL = harness.server.URL
	instance.client.http = harness.server.Client()
	return instance
}

// accountStatusValues 调一次 /account/status 并取出响应值。
func accountStatusValues(t *testing.T, instance *plugin) map[string]any {
	t.Helper()
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeAccountStatusID, nil))
	if err != nil {
		t.Fatalf("查询账号状态失败：%v", err)
	}
	return routeDocumentValues(t, response, accountStatusOutputSchema)
}

// /account/status：机器人身份、网关状态与群数量。
func TestRouteAccountStatusReportsGatewayAndGroups(t *testing.T) {
	config := testConfig()
	config.AppID = "1906413794"
	// 重复项与空白项都不该计入白名单条数。
	config.GroupIDs = []string{"GROUP_A", "GROUP_B", "GROUP_A", "   "}
	harness := newAccountStatusHarness(t, http.StatusOK, map[string]any{
		"id": "5777414462219517083", "username": "SForum",
		"avatar": "https://thirdqq.qlogo.cn/g?b=oidb&k=abc&s=0", "bot": true,
	})
	instance := newAccountStatusPlugin(t, config, harness)
	instance.state.RecordClaim("GROUP_A", "GROUP_AT_MESSAGE_CREATE")

	values := accountStatusValues(t, instance)

	if values["appId"] != "1906413794" {
		t.Fatalf("appId 不对：%v", values["appId"])
	}
	if values["credentialsConfigured"] != true {
		t.Fatalf("凭据齐全时 credentialsConfigured 应为 true：%v", values["credentialsConfigured"])
	}
	bot, ok := values["bot"].(map[string]any)
	if !ok {
		t.Fatalf("bot 不是对象：%T", values["bot"])
	}
	if bot["name"] != "SForum" {
		t.Fatalf("机器人昵称不对：%v", bot["name"])
	}
	if bot["avatarUrl"] != "https://thirdqq.qlogo.cn/g?b=oidb&k=abc&s=0" {
		t.Fatalf("头像应是平台返回的完整 URL：%v", bot["avatarUrl"])
	}
	if bot["available"] != true || bot["reason"] != nil {
		t.Fatalf("信息可用时 available 应为 true 且 reason 为 null：%v / %v", bot["available"], bot["reason"])
	}

	gateway, ok := values["gateway"].(map[string]any)
	if !ok {
		t.Fatalf("gateway 不是对象：%T", values["gateway"])
	}
	if gateway["state"] != gatewayStateDisabled {
		t.Fatalf("未开启群 ID 识别时网关应为 disabled，得到 %v", gateway["state"])
	}
	if gateway["reason"] != "" {
		t.Fatalf("没有错误时 reason 应为空串：%v", gateway["reason"])
	}
	if gateway["since"] != nil {
		t.Fatalf("未连接时 since 应为 null：%v", gateway["since"])
	}

	groups, ok := values["groups"].(map[string]any)
	if !ok {
		t.Fatalf("groups 不是对象：%T", values["groups"])
	}
	if recognized, ok := groups["recognized"].(float64); !ok || int(recognized) != 1 {
		t.Fatalf("recognized 应为 1，得到 %v", groups["recognized"])
	}
	if whitelisted, ok := groups["whitelisted"].(float64); !ok || int(whitelisted) != 2 {
		t.Fatalf("whitelisted 应为去重后的 2，得到 %v", groups["whitelisted"])
	}
	if checkedAt, _ := values["checkedAt"].(string); checkedAt == "" {
		t.Fatal("checkedAt 缺失")
	} else if _, err := time.Parse(time.RFC3339, checkedAt); err != nil {
		t.Fatalf("checkedAt 不是 RFC 3339：%q", checkedAt)
	}
}

// /account/status：机器人信息拿不到时降级成 available=false + 可读原因，仍 200。
func TestRouteAccountStatusBotInfoUnavailableDegrades(t *testing.T) {
	config := testConfig()
	harness := newAccountStatusHarness(t, http.StatusUnauthorized, map[string]any{
		"err_code": 100016, "message": "invalid appid or secret",
	})
	instance := newAccountStatusPlugin(t, config, harness)

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeAccountStatusID, nil))
	if err != nil {
		t.Fatalf("机器人信息失败不该让路由返回 error：%v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("降级时仍应 200，得到 %d", response.GetStatusCode())
	}
	values := routeDocumentValues(t, response, accountStatusOutputSchema)
	bot, _ := values["bot"].(map[string]any)
	if bot["available"] != false {
		t.Fatalf("拿不到信息时 available 应为 false：%v", bot["available"])
	}
	reason, _ := bot["reason"].(string)
	if !strings.Contains(reason, "凭据无效") {
		t.Fatalf("reason 应是可读原因：%q", reason)
	}
	if bot["name"] != "" || bot["avatarUrl"] != "" {
		t.Fatalf("不可用时应给空昵称与空头像：%v / %v", bot["name"], bot["avatarUrl"])
	}
	if groups, _ := values["groups"].(map[string]any); groups == nil {
		t.Fatal("机器人信息失败不该影响 groups 块")
	}
}

// /account/status：机器人信息缓存 10 分钟，两次调用只打一次 /users/@me。
func TestRouteAccountStatusCachesBotInfo(t *testing.T) {
	config := testConfig()
	harness := newAccountStatusHarness(t, http.StatusOK, map[string]any{
		"id": "1", "username": "SForum", "avatar": "https://thirdqq.qlogo.cn/g?b=oidb&k=abc", "bot": true,
	})
	instance := newAccountStatusPlugin(t, config, harness)

	accountStatusValues(t, instance)
	accountStatusValues(t, instance)

	if calls := harness.calls(); calls != 1 {
		t.Fatalf("10 分钟内应命中缓存，实际请求 %d 次", calls)
	}
}

// /account/status：零凭据时一个请求都不发，只回报不可用。
func TestRouteAccountStatusWithoutCredentialsSkipsRequest(t *testing.T) {
	config := testConfig()
	config.AppSecret = ""
	harness := newAccountStatusHarness(t, http.StatusOK, map[string]any{"username": "SForum"})
	instance := newAccountStatusPlugin(t, config, harness)

	values := accountStatusValues(t, instance)

	if calls := harness.calls(); calls != 0 {
		t.Fatalf("零凭据不该发任何请求，实际 %d 次", calls)
	}
	if values["credentialsConfigured"] != false {
		t.Fatalf("缺凭据时 credentialsConfigured 应为 false：%v", values["credentialsConfigured"])
	}
	bot, _ := values["bot"].(map[string]any)
	if bot["available"] != false {
		t.Fatalf("零凭据时 available 应为 false：%v", bot["available"])
	}
	if reason, _ := bot["reason"].(string); !strings.Contains(reason, "凭据") {
		t.Fatalf("reason 应说明缺凭据：%q", reason)
	}
}
