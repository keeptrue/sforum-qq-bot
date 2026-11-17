package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// groupsListServer 造一个假的 QQ OpenAPI：取凭证恒成功，群信息按 info 决定。
func groupsListServer(t *testing.T, info func(writer http.ResponseWriter, groupOpenID string)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var infoCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == tokenPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-groups", "expires_in": 7200})
		case strings.HasPrefix(request.URL.Path, groupPathPrefix) && strings.HasSuffix(request.URL.Path, "/info"):
			infoCalls.Add(1)
			openID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, groupPathPrefix), "/info")
			info(writer, openID)
		default:
			t.Errorf("未预期路径：%s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	}))
	t.Cleanup(server.Close)
	return server, &infoCalls
}

// groupsListGroup 从响应里取出第 index 个群。
func groupsListGroup(t *testing.T, values map[string]any, index int) map[string]any {
	t.Helper()
	groups, ok := values["groups"].([]any)
	if !ok {
		t.Fatalf("groups 不是数组：%T %v", values["groups"], values["groups"])
	}
	if index >= len(groups) {
		t.Fatalf("groups 只有 %d 个，取不到第 %d 个：%v", len(groups), index, groups)
	}
	group, ok := groups[index].(map[string]any)
	if !ok {
		t.Fatalf("第 %d 个群不是对象：%T", index, groups[index])
	}
	return group
}

// groups/list：把已识别的群列出来，群名能拿到就填上。
func TestRouteGroupsListReturnsClaims(t *testing.T) {
	config := testConfig()
	config.EnableGroupIDClaim = true
	instance := newRouteTestPlugin(t, config, nil)
	server, infoCalls := groupsListServer(t, func(writer http.ResponseWriter, groupOpenID string) {
		writeJSON(t, writer, http.StatusOK, map[string]any{
			"group_openid": groupOpenID, "group_name": "群名-" + groupOpenID, "group_member_num": 42,
		})
	})
	instance.client.baseURL = server.URL

	instance.state.RecordClaim("GROUP_A", "GROUP_AT_MESSAGE_CREATE")
	instance.state.RecordClaim("GROUP_B", "GROUP_AT_MESSAGE_CREATE")

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeGroupsListID, nil))
	if err != nil {
		t.Fatalf("查询群列表失败：%v", err)
	}
	values := routeDocumentValues(t, response, groupsListOutputSchema)
	if groups, _ := values["groups"].([]any); len(groups) != 2 {
		t.Fatalf("应有 2 个群，实际 %v", values["groups"])
	}
	first := groupsListGroup(t, values, 0)
	if first["openid"] != "GROUP_A" {
		t.Fatalf("第一个群的 openid 不对：%v", first["openid"])
	}
	if first["name"] != "群名-GROUP_A" {
		t.Fatalf("群名应填上，得到 %v", first["name"])
	}
	if first["nameUnavailable"] != false {
		t.Fatalf("拿到名字时 nameUnavailable 应为 false：%v", first["nameUnavailable"])
	}
	if first["eventType"] != "GROUP_AT_MESSAGE_CREATE" {
		t.Fatalf("eventType 不对：%v", first["eventType"])
	}
	if count, ok := first["count"].(float64); !ok || count != 1 {
		t.Fatalf("count 不对：%v", first["count"])
	}
	for _, key := range []string{"firstSeenAt", "lastSeenAt"} {
		text, _ := first[key].(string)
		if _, parseErr := time.Parse(time.RFC3339, text); parseErr != nil {
			t.Fatalf("%s 不是 RFC 3339：%q", key, text)
		}
	}
	if values["claimEnabled"] != true {
		t.Fatalf("claimEnabled 应读设置项：%v", values["claimEnabled"])
	}
	if limit, ok := values["recordingLimit"].(float64); !ok || int(limit) != claimRingSize {
		t.Fatalf("recordingLimit 应为环形容量 %d，得到 %v", claimRingSize, values["recordingLimit"])
	}
	if infoCalls.Load() != 2 {
		t.Fatalf("两个群各查一次群名，实际 %d 次", infoCalls.Load())
	}
}

// groups/list：群信息接口回 11253（普通应用没有白名单）时，列表照常返回，
// 只是名字为空并标 nameUnavailable；失败结果还要被缓存，别每次刷新都打平台。
func TestRouteGroupsListNameUnavailableDegrades(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	server, infoCalls := groupsListServer(t, func(writer http.ResponseWriter, _ string) {
		// 平台在 HTTP 200 里返回业务失败，正是群信息接口的真实形态。
		writeJSON(t, writer, http.StatusOK, map[string]any{
			"err_code": 11253, "message": "应用无接口访问权限",
		})
	})
	instance.client.baseURL = server.URL
	instance.state.RecordClaim("GROUP_A", "GROUP_AT_MESSAGE_CREATE")

	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeGroupsListID, nil))
	if err != nil {
		t.Fatalf("群名失败不该让列表接口返回 error：%v", err)
	}
	values := routeDocumentValues(t, response, groupsListOutputSchema)
	group := groupsListGroup(t, values, 0)
	if group["openid"] != "GROUP_A" {
		t.Fatalf("拿不到名字也必须保留 openid：%v", group)
	}
	if group["name"] != nil {
		t.Fatalf("拿不到名字时 name 应为 null，得到 %v", group["name"])
	}
	if group["nameUnavailable"] != true {
		t.Fatalf("nameUnavailable 应为 true：%v", group["nameUnavailable"])
	}

	// 再查一次：失败结果在 TTL 内命中缓存，不该再打平台。
	if _, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeGroupsListID, nil)); err != nil {
		t.Fatalf("第二次查询失败：%v", err)
	}
	if infoCalls.Load() != 1 {
		t.Fatalf("失败结果应被缓存，实际查了 %d 次", infoCalls.Load())
	}
}

// groups/list 的权限面由宿主守卫执行（core.guard.permission + 插件权限键），
// 插件侧不该复读一遍判定；这里钉住的是清单声明，改错就会在宿主侧变成另一个权限面。
func TestRouteGroupsListRequiresPermission(t *testing.T) {
	manifest := loadExtensionManifest(t)
	route, found := findManifestRoute(manifest.Routes, routeGroupsListID)
	if !found {
		t.Fatalf("清单缺少路由 %s", routeGroupsListID)
	}
	if route.Guard != "core.guard.permission" {
		t.Fatalf("守卫应为 core.guard.permission，清单写的是 %q", route.Guard)
	}
	if route.Permission != extensionID+".manage" {
		t.Fatalf("权限键应为 %s.manage，清单写的是 %q", extensionID, route.Permission)
	}
	if route.Access != "permission" {
		t.Fatalf("access 应为 permission，清单写的是 %q", route.Access)
	}
	if route.Fallback != "closed" {
		t.Fatalf("fallback 应为 closed，清单写的是 %q", route.Fallback)
	}
	if route.Path != "/groups/list" {
		t.Fatalf("路径应为 /groups/list，清单写的是 %q", route.Path)
	}
	if len(route.Methods) != 1 || route.Methods[0] != "POST" {
		t.Fatalf("只应声明 POST，清单写的是 %v", route.Methods)
	}
}

// groups/list：一个群都没识别到时，groups 必须是空数组而不是 null。
func TestRouteGroupsListEmpty(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	response, err := instance.InvokeRoute(context.Background(), newRouteRequest(t, routeGroupsListID, nil))
	if err != nil {
		t.Fatalf("查询空列表失败：%v", err)
	}
	values := routeDocumentValues(t, response, groupsListOutputSchema)
	groups, ok := values["groups"].([]any)
	if !ok {
		t.Fatalf("groups 必须是数组（不能是 null）：%T %v", values["groups"], values["groups"])
	}
	if len(groups) != 0 {
		t.Fatalf("没有群时 groups 应为空，实际 %v", groups)
	}
	if limit, ok := values["recordingLimit"].(float64); !ok || int(limit) != claimRingSize {
		t.Fatalf("recordingLimit 应为 %d，得到 %v", claimRingSize, values["recordingLimit"])
	}
}
