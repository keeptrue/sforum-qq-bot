package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// readGatewayFrame 在超时内读一帧。
func readGatewayFrame(conn *websocket.Conn, timeout time.Duration) (gatewayFrame, error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return gatewayFrame{}, err
	}
	var frame gatewayFrame
	if err := json.Unmarshal(message, &frame); err != nil {
		return gatewayFrame{}, err
	}
	return frame, nil
}

// testGatewayHarness 起一个同时提供 HTTP API 与 WebSocket 的假网关。
type testGatewayHarness struct {
	server    *httptest.Server
	connCount atomic.Int32
}

func newGatewayHarness(t *testing.T, scripts []func(conn *websocket.Conn)) *testGatewayHarness {
	t.Helper()
	harness := &testGatewayHarness{}
	upgrader := websocket.Upgrader{}
	var wsURL string

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case tokenPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-1", "expires_in": 7200})
		case gatewayPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"url": wsURL})
		case "/ws":
			conn, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				t.Errorf("升级 WebSocket 失败：%v", err)
				return
			}
			index := int(harness.connCount.Add(1)) - 1
			if index < len(scripts) {
				scripts[index](conn)
			}
			_ = conn.Close()
		default:
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	})
	harness.server = httptest.NewServer(handler)
	wsURL = "ws" + harness.server.URL[len("http"):] + "/ws"
	t.Cleanup(harness.server.Close)
	return harness
}

func newTestGateway(t *testing.T, harness *testGatewayHarness) (*Gateway, *State) {
	t.Helper()
	config := testConfig()
	config.EnableGroupIDClaim = true
	client := newQQClient(config, testLogger())
	client.baseURL = harness.server.URL
	client.http = harness.server.Client()
	state := NewState()
	gateway := newGateway(config, client, state, testLogger())
	gateway.backoffBase = 5 * time.Millisecond
	gateway.unauthorizedBackoff = 20 * time.Millisecond
	gateway.helloTimeout = 2 * time.Second
	t.Cleanup(gateway.Stop)
	return gateway, state
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func decodeIdentify(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Errorf("Identify payload 不是 JSON：%v", err)
	}
	return payload
}

func TestGatewayIdentifyHeartbeatAndResume(t *testing.T) {
	harness := newGatewayHarness(t, []func(conn *websocket.Conn){
		// 第一条连接：Hello → Identify → READY → 群事件 → 心跳 → 4009 断开。
		func(conn *websocket.Conn) {
			if err := conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 40}}); err != nil {
				t.Errorf("写 Hello 失败：%v", err)
				return
			}
			frame, err := readGatewayFrame(conn, 2*time.Second)
			if err != nil {
				t.Errorf("没收到 Identify：%v", err)
				return
			}
			if frame.Op != gatewayOpIdentify {
				t.Errorf("首个帧应是 Identify(2)，得到 %d", frame.Op)
			}
			identify := decodeIdentify(t, frame.D)
			if identify["token"] != "QQBot tok-1" {
				t.Errorf("Identify token 不对：%v", identify["token"])
			}
			if intents, _ := identify["intents"].(float64); int(intents) != groupIntents {
				t.Errorf("Identify intents 应为 %d，得到 %v", groupIntents, identify["intents"])
			}
			// 还没收到任何带 s 的帧：首次心跳的 d 必须是 null。
			firstHeartbeat, err := readGatewayFrame(conn, 2*time.Second)
			if err != nil {
				t.Errorf("没收到首次心跳：%v", err)
				return
			}
			if firstHeartbeat.Op != gatewayOpHeartbeat {
				t.Errorf("应收到心跳(1)，得到 %d", firstHeartbeat.Op)
			}
			if string(firstHeartbeat.D) != "null" {
				t.Errorf("首次心跳 d 应为 null，得到 %s", string(firstHeartbeat.D))
			}
			if err := conn.WriteJSON(map[string]any{
				"op": 0, "s": 1, "t": "READY", "d": map[string]any{"session_id": "sess-1"},
			}); err != nil {
				t.Errorf("写 READY 失败：%v", err)
				return
			}
			if err := conn.WriteJSON(map[string]any{
				"op": 0, "s": 2, "t": "GROUP_ADD_ROBOT",
				"d": map[string]any{"group_openid": "GROUP123456"},
			}); err != nil {
				t.Errorf("写群事件失败：%v", err)
				return
			}
			// 收到事件之后的心跳应带上最近 s。
			heartbeat, err := readGatewayFrame(conn, 2*time.Second)
			if err != nil {
				t.Errorf("没收到心跳：%v", err)
				return
			}
			if heartbeat.Op != gatewayOpHeartbeat {
				t.Errorf("应收到心跳(1)，得到 %d", heartbeat.Op)
			}
			if string(heartbeat.D) != "2" {
				t.Errorf("收到事件后的心跳应带最近 seq=2，得到 %s", string(heartbeat.D))
			}
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(4009, "session expired"), time.Now().Add(time.Second))
		},
		// 第二条连接：必须 Resume，而不是重新 Identify。
		func(conn *websocket.Conn) {
			if err := conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 40}}); err != nil {
				t.Errorf("写第二个 Hello 失败：%v", err)
				return
			}
			frame, err := readGatewayFrame(conn, 2*time.Second)
			if err != nil {
				t.Errorf("没收到 Resume：%v", err)
				return
			}
			if frame.Op != gatewayOpResume {
				t.Errorf("4009 之后应发 Resume(6)，得到 %d", frame.Op)
			}
			var payload map[string]any
			_ = json.Unmarshal(frame.D, &payload)
			if payload["session_id"] != "sess-1" {
				t.Errorf("Resume 应带 session_id=sess-1，得到 %v", payload["session_id"])
			}
			if seq, _ := payload["seq"].(float64); int(seq) != 2 {
				t.Errorf("Resume 应带最近 seq=2，得到 %v", payload["seq"])
			}
			if err := conn.WriteJSON(map[string]any{"op": 0, "s": 3, "t": "RESUMED", "d": ""}); err != nil {
				t.Errorf("写 RESUMED 失败：%v", err)
			}
			// 保持连接直到测试 stop。
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		},
	})

	gateway, state := newTestGateway(t, harness)
	gateway.Start()

	if !waitFor(t, 3*time.Second, func() bool { return gateway.Status().State == gatewayStateConnected }) {
		t.Fatalf("长连接应进入 connected，当前 %+v", gateway.Status())
	}
	claims := state.RecentClaims(10)
	if len(claims) != 1 || claims[0].GroupID != "GROUP123456" {
		t.Fatalf("应识别到群 GROUP123456，得到 %+v", claims)
	}
	if claims[0].EventType != "GROUP_ADD_ROBOT" {
		t.Fatalf("事件类型不对：%q", claims[0].EventType)
	}
	if !waitFor(t, 3*time.Second, func() bool { return harness.connCount.Load() >= 2 }) {
		t.Fatalf("4009 之后应重连，连接数 %d", harness.connCount.Load())
	}
	// 第二条连接必须完成 Resume（脚本里断言），状态应回到 connected。
	if !waitFor(t, 3*time.Second, func() bool { return gateway.Status().State == gatewayStateConnected }) {
		t.Fatalf("Resume 后应回到 connected，当前 %+v", gateway.Status())
	}

	gateway.Stop()
	select {
	case <-gateway.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 后 supervisor 应退出")
	}
}

func TestGatewayUnauthorizedBacksOffAndReIdentifies(t *testing.T) {
	var secondFrameOp atomic.Int32
	harness := newGatewayHarness(t, []func(conn *websocket.Conn){
		func(conn *websocket.Conn) {
			_ = conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 1000}})
			if _, err := readGatewayFrame(conn, 2*time.Second); err != nil {
				t.Errorf("没收到 Identify：%v", err)
				return
			}
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(4014, "intent no permission"), time.Now().Add(time.Second))
		},
		func(conn *websocket.Conn) {
			_ = conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 1000}})
			frame, err := readGatewayFrame(conn, 2*time.Second)
			if err != nil {
				t.Errorf("重试连接没有收到帧：%v", err)
				return
			}
			secondFrameOp.Store(int32(frame.Op))
			if err := conn.WriteJSON(map[string]any{"op": 0, "s": 1, "t": "READY", "d": map[string]any{"session_id": "sess-2"}}); err != nil {
				t.Errorf("写 READY 失败：%v", err)
				return
			}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		},
	})

	gateway, _ := newTestGateway(t, harness)
	gateway.Start()

	if !waitFor(t, 3*time.Second, func() bool { return gateway.Status().State == gatewayStateUnauthorized }) {
		t.Fatalf("4014 应进入 unauthorized，当前 %+v", gateway.Status())
	}
	if reason := gateway.Status().LastError; reason != "平台未授权群事件订阅" {
		t.Fatalf("状态应说明未授权，得到 %q", reason)
	}
	if !waitFor(t, 3*time.Second, func() bool { return harness.connCount.Load() >= 2 }) {
		t.Fatalf("退避后应重试连接，连接数 %d", harness.connCount.Load())
	}
	if !waitFor(t, 3*time.Second, func() bool { return secondFrameOp.Load() != 0 }) {
		t.Fatal("第二次连接没有发出帧")
	}
	if op := secondFrameOp.Load(); op != gatewayOpIdentify {
		t.Fatalf("4014 之后应重新 Identify(2)，得到 %d", op)
	}
	if !waitFor(t, 3*time.Second, func() bool { return gateway.Status().State == gatewayStateConnected }) {
		t.Fatalf("重新 Identify 后应 connected，当前 %+v", gateway.Status())
	}
	gateway.Stop()
}

func TestGatewayProbeOnlyShutsDownAndStopsReconnecting(t *testing.T) {
	harness := newGatewayHarness(t, []func(conn *websocket.Conn){
		func(conn *websocket.Conn) {
			_ = conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 1000}})
			if _, err := readGatewayFrame(conn, 2*time.Second); err != nil {
				t.Errorf("没收到 Identify：%v", err)
				return
			}
			_ = conn.WriteJSON(map[string]any{"op": 0, "s": 1, "t": "READY", "d": map[string]any{"session_id": "sess-probe"}})
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		},
	})
	gateway, _ := newTestGateway(t, harness)
	gateway.Start()
	if !waitFor(t, 3*time.Second, func() bool { return gateway.Status().State == gatewayStateConnected }) {
		t.Fatalf("应先连上，当前 %+v", gateway.Status())
	}

	gateway.MarkProbeOnly()

	select {
	case <-gateway.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("探测隔离应关闭 supervisor")
	}
	if state := gateway.Status().State; state != gatewayStateDisabled {
		t.Fatalf("探测隔离后状态应为 disabled，得到 %q", state)
	}
	connections := harness.connCount.Load()
	time.Sleep(50 * time.Millisecond)
	if harness.connCount.Load() != connections {
		t.Fatalf("探测隔离后不应重连，连接数从 %d 变成 %d", connections, harness.connCount.Load())
	}
}

func TestGatewayDisabledWithoutCredentials(t *testing.T) {
	config := testConfig()
	config.AppID = ""
	config.AppSecret = ""
	client := newQQClient(config, testLogger())
	gateway := newGateway(config, client, NewState(), testLogger())
	gateway.Start()
	if state := gateway.Status().State; state != gatewayStateDisabled {
		t.Fatalf("零配置下应为 disabled，得到 %q", state)
	}
	select {
	case <-gateway.Done():
		t.Fatal("未配置时不应启动 supervisor")
	default:
	}
}

func TestGatewayStatusReportsUptime(t *testing.T) {
	config := testConfig()
	config.EnableGroupIDClaim = true
	base := time.Now()
	current := base
	gateway := newGateway(config, nil, NewState(), testLogger())
	gateway.now = func() time.Time { return current }
	gateway.setState(gatewayStateConnected, "")
	current = base.Add(90 * time.Second)
	status := gateway.Status()
	if status.State != gatewayStateConnected {
		t.Fatalf("状态不对：%q", status.State)
	}
	if status.ConnectedFor != 90*time.Second {
		t.Fatalf("连接时长应为 90s，得到 %v", status.ConnectedFor)
	}
}

// mentionReplyHarness 是一个只关心「群消息发送」的假 QQ API：取凭证恒成功，
// 消息接口记录请求体，并可按 failErr 返回业务失败。
type mentionReplyHarness struct {
	server  *httptest.Server
	mu      sync.Mutex
	bodies  []map[string]any
	failErr int
}

func newMentionReplyHarness(t *testing.T) *mentionReplyHarness {
	t.Helper()
	harness := &mentionReplyHarness{}
	harness.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == tokenPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-1", "expires_in": 7200})
		case strings.HasPrefix(request.URL.Path, "/v2/groups/") && strings.HasSuffix(request.URL.Path, "/messages"):
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("消息请求体不是 JSON：%v", err)
			}
			harness.mu.Lock()
			harness.bodies = append(harness.bodies, body)
			failErr := harness.failErr
			harness.mu.Unlock()
			if failErr != 0 {
				// 平台在 HTTP 200 里报业务失败，正是被动回复过期的真实形态。
				writeJSON(t, writer, http.StatusOK, map[string]any{"err_code": failErr, "message": "passive reply expired"})
				return
			}
			writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg-out-1"})
		default:
			t.Errorf("意外路径：%s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	}))
	t.Cleanup(harness.server.Close)
	return harness
}

func (h *mentionReplyHarness) hits() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.bodies)
}

func (h *mentionReplyHarness) lastBody() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.bodies) == 0 {
		return nil
	}
	return h.bodies[len(h.bodies)-1]
}

// newMentionReplyGateway 组装一个只用于 @ 回复的网关：真 QQClient 指向假 API，
// 群 ID 认领与回复都走同一条 handleGroupEvent。
func newMentionReplyGateway(t *testing.T, config Config, harness *mentionReplyHarness) (*Gateway, *State) {
	t.Helper()
	client := newQQClient(config, testLogger())
	client.baseURL = harness.server.URL
	client.http = harness.server.Client()
	state := NewState()
	gateway := newGateway(config, client, state, testLogger())
	return gateway, state
}

// 一条 @ 事件：msg_id 在 d.id 上，群在 group_openid 上。
func mentionEventJSON(msgID string) json.RawMessage {
	return json.RawMessage(`{"id":"` + msgID + `","group_openid":"GROUP_A","content":"@SForum hi",` +
		`"author":{"member_openid":"member-1"}}`)
}

// 被 @ 时用事件里的 msg_id 做被动回复（带 msg_seq=1），群 ID 同时被认领。
func TestGatewayRepliesToMentionWithMsgID(t *testing.T) {
	config := testConfig()
	config.ReplyOnMention = true
	harness := newMentionReplyHarness(t)
	gateway, state := newMentionReplyGateway(t, config, harness)

	gateway.handleGroupEvent("GROUP_AT_MESSAGE_CREATE", mentionEventJSON("msg-in-1"))

	if hits := harness.hits(); hits != 1 {
		t.Fatalf("应被动回复一次，实际 %d 次", hits)
	}
	body := harness.lastBody()
	if body["msg_id"] != "msg-in-1" {
		t.Fatalf("msg_id 应取自事件的 d.id，得到 %v", body["msg_id"])
	}
	if seq, ok := body["msg_seq"].(float64); !ok || int(seq) != 1 {
		t.Fatalf("msg_seq 应固定为 1，得到 %v", body["msg_seq"])
	}
	if body["content"] != mentionReplyText {
		t.Fatalf("回复文案不对：%v", body["content"])
	}
	if format, ok := body["msg_type"].(float64); !ok || int(format) != 0 {
		t.Fatalf("被动回复应是纯文本 msg_type=0，得到 %v", body["msg_type"])
	}

	claims := state.RecentClaims(claimRingSize)
	if len(claims) != 1 || claims[0].GroupID != "GROUP_A" {
		t.Fatalf("群 ID 应同时被认领：%+v", claims)
	}
}

// 同一个群 60 秒内只回一次；但群 ID 认领每次都记（认领是主功能）。
func TestGatewayMentionReplyDeduplicated(t *testing.T) {
	config := testConfig()
	config.ReplyOnMention = true
	harness := newMentionReplyHarness(t)
	gateway, state := newMentionReplyGateway(t, config, harness)

	now := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	gateway.now = func() time.Time { return now }

	gateway.handleGroupEvent("GROUP_AT_MESSAGE_CREATE", mentionEventJSON("msg-in-1"))
	now = now.Add(30 * time.Second)
	gateway.handleGroupEvent("GROUP_AT_MESSAGE_CREATE", mentionEventJSON("msg-in-2"))

	if hits := harness.hits(); hits != 1 {
		t.Fatalf("60 秒内只该回一次，实际 %d 次", hits)
	}
	claims := state.RecentClaims(claimRingSize)
	if len(claims) != 1 || claims[0].Count != 2 {
		t.Fatalf("两次事件都该计入群 ID 认领：%+v", claims)
	}

	// 过了冷却时间就再放行一次。
	now = now.Add(31 * time.Second)
	gateway.handleGroupEvent("GROUP_AT_MESSAGE_CREATE", mentionEventJSON("msg-in-3"))
	if hits := harness.hits(); hits != 2 {
		t.Fatalf("超过 60 秒应再次回复，实际 %d 次", hits)
	}
}

// 开关关闭时不发任何消息，但群 ID 照记。
func TestGatewayMentionReplyDisabled(t *testing.T) {
	config := testConfig()
	config.ReplyOnMention = false
	harness := newMentionReplyHarness(t)
	gateway, state := newMentionReplyGateway(t, config, harness)

	gateway.handleGroupEvent("GROUP_AT_MESSAGE_CREATE", mentionEventJSON("msg-in-1"))

	if hits := harness.hits(); hits != 0 {
		t.Fatalf("开关关闭时不该发送，实际 %d 次", hits)
	}
	if claims := state.RecentClaims(claimRingSize); len(claims) != 1 || claims[0].GroupID != "GROUP_A" {
		t.Fatalf("群 ID 仍应被认领：%+v", claims)
	}
}

// 回复失败（msg_id 过期）只记日志：群 ID 认领不能被它拖累。
func TestGatewayMentionReplyFailureKeepsClaim(t *testing.T) {
	config := testConfig()
	config.ReplyOnMention = true
	harness := newMentionReplyHarness(t)
	harness.failErr = 40034005
	gateway, state := newMentionReplyGateway(t, config, harness)

	gateway.handleGroupEvent("GROUP_AT_MESSAGE_CREATE", mentionEventJSON("msg-in-1"))

	if hits := harness.hits(); hits != 1 {
		t.Fatalf("应尝试回复一次，实际 %d 次", hits)
	}
	claims := state.RecentClaims(claimRingSize)
	if len(claims) != 1 || claims[0].GroupID != "GROUP_A" {
		t.Fatalf("回复失败不该影响群 ID 认领：%+v", claims)
	}
	if reason := errorForCode(40034005, "", http.StatusOK).AdminMessage(); !strings.Contains(reason, "被动回复窗口") {
		t.Fatalf("40034005 应翻译成被动回复过期：%q", reason)
	}
}
