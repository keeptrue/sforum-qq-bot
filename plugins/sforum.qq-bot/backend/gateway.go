package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// 网关 opcode（QQ 官方 payload 定义）。
const (
	gatewayOpDispatch       = 0
	gatewayOpHeartbeat      = 1
	gatewayOpIdentify       = 2
	gatewayOpResume         = 6
	gatewayOpReconnect      = 7
	gatewayOpInvalidSession = 9
	gatewayOpHello          = 10
	gatewayOpHeartbeatACK   = 11
)

// groupIntents = 1 << 25（群与 C2C 事件位）：GROUP_ADD_ROBOT /
// GROUP_AT_MESSAGE_CREATE / GROUP_MSG_RECEIVE 都挂在这一位下。
const groupIntents = 1 << 25

// 状态机取值，供 status 工具直接展示。
const (
	gatewayStateDisabled     = "disabled"
	gatewayStateConnecting   = "connecting"
	gatewayStateConnected    = "connected"
	gatewayStateUnauthorized = "unauthorized"
	gatewayStateError        = "error"
)

// 断线处置分类。
const (
	faultResume       = "resume"
	faultIdentify     = "identify"
	faultUnauthorized = "unauthorized"
	faultBackoff      = "backoff"
)

// claimEvents 是会被写入「已识别群 ID」的事件名。
var claimEvents = map[string]struct{}{
	"GROUP_ADD_ROBOT":         {},
	"GROUP_AT_MESSAGE_CREATE": {},
	"GROUP_MSG_RECEIVE":       {},
}

const (
	// mentionReplyCooldown 是同一个群两次被动回复的最小间隔。平台侧被动回复有
	// 5 分钟窗口，但运营者可能连着 @ 几次；60 秒足够表达「收到了」，又不会刷屏。
	mentionReplyCooldown = 60 * time.Second
	// mentionReplyTimeout 是单次被动回复的等待上限。网关读循环在这段时间里不读
	// 新帧，所以刻意取得比发送超时短：宁可这一条回不出去，也别卡住事件处理。
	mentionReplyTimeout = 5 * time.Second
)

// mentionReplyText 是被 @ 时的固定回复：只说清「这个机器人做什么、这个群怎么
// 变成推送目标」。它刻意不是设置项——文案可配置就多一套运营面，而这句话不该
// 有第二种说法。
const mentionReplyText = "本机器人只做 SForum 论坛新帖推送；这个群已记录，可在后台勾选为推送目标。"

// GatewayStatus 是长连接的可读状态。
type GatewayStatus struct {
	State        string
	LastError    string
	ConnectedFor time.Duration
}

type gatewayFault struct {
	kind   string
	reason string
}

type gatewayFrame struct {
	Op int             `json:"op"`
	S  int64           `json:"s"`
	T  string          `json:"t"`
	D  json.RawMessage `json:"d"`
}

// Gateway 是群 ID 识别通道：一条出站长连接，只认「机器人被拉进群」「群里 @机器人」
// 这类事件，从中取出 group_openid 供管理员复制到白名单。
//
// 它是可选能力：凭据不全或开关关闭时根本不启动，进程零配置也不会建立任何连接。
type Gateway struct {
	cfg    Config
	client *QQClient
	state  *State
	log    *logger

	dialer   *websocket.Dialer
	endpoint func(ctx context.Context) (string, error)

	stopCh    chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	probeOnly atomic.Bool
	writeMu   sync.Mutex

	mu          sync.Mutex
	status      string
	lastError   string
	connectedAt time.Time
	sessionID   string
	seq         int64
	conn        *websocket.Conn

	now                 func() time.Time
	backoffBase         time.Duration
	backoffMax          time.Duration
	unauthorizedBackoff time.Duration
	helloTimeout        time.Duration

	// mentionGate 是「同群 60 秒只回一次」的进程内记录。
	mentionGate *mentionReplyGate
}

func newGateway(cfg Config, client *QQClient, state *State, log *logger) *Gateway {
	gateway := &Gateway{
		cfg:                 cfg,
		client:              client,
		state:               state,
		log:                 log,
		dialer:              websocket.DefaultDialer,
		stopCh:              make(chan struct{}),
		done:                make(chan struct{}),
		status:              gatewayStateDisabled,
		now:                 time.Now,
		backoffBase:         time.Second,
		backoffMax:          time.Minute,
		unauthorizedBackoff: 5 * time.Minute,
		helloTimeout:        10 * time.Second,
		mentionGate:         newMentionReplyGate(),
	}
	if cfg.GroupIDClaimEnabled() {
		gateway.status = gatewayStateConnecting
		gateway.endpoint = client.GatewayURL
	}
	return gateway
}

// Start 启动 supervisor：一次连接失败不会让它退出，它按错误分类退避重连。
func (g *Gateway) Start() {
	if g == nil {
		return
	}
	if !g.cfg.GroupIDClaimEnabled() {
		g.setState(gatewayStateDisabled, "")
		return
	}
	g.startOnce.Do(func() { go g.run() })
}

// Stop 关闭长连接并让 supervisor 退出（不再重连）。
func (g *Gateway) Stop() {
	if g == nil {
		return
	}
	g.stopOnce.Do(func() { close(g.stopCh) })
	g.closeConn()
}

// Done 在 supervisor 退出后关闭，供测试与进程退出时等待。
func (g *Gateway) Done() <-chan struct{} {
	if g == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return g.done
}

// MarkProbeOnly 是「探测隔离」的唯一入口：设置页按钮会用一个短命 runtime 调
// 工具，它没有 Host token，绝不能抢走正式连接的 session。第一次调用工具时置位
// 并关闭长连接，之后 supervisor 不再重连。
func (g *Gateway) MarkProbeOnly() {
	if g == nil {
		return
	}
	g.probeOnly.Store(true)
	g.Stop()
	g.setState(gatewayStateDisabled, "探测运行时已关闭长连接")
}

// Status 返回当前状态与最近一次错误。
func (g *Gateway) Status() GatewayStatus {
	if g == nil {
		return GatewayStatus{State: gatewayStateDisabled}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	status := GatewayStatus{State: g.status, LastError: g.lastError}
	if g.status == gatewayStateConnected && !g.connectedAt.IsZero() {
		status.ConnectedFor = g.now().Sub(g.connectedAt)
	}
	return status
}

func (g *Gateway) run() {
	defer close(g.done)
	backoff := g.backoffBase
	for {
		if g.stopped() {
			return
		}
		if g.probeOnly.Load() {
			return
		}
		g.setState(gatewayStateConnecting, "")
		fault, connected := g.session()
		if connected {
			backoff = g.backoffBase
		}
		if g.stopped() || g.probeOnly.Load() {
			return
		}
		wait := fault.delay(g, backoff)
		switch fault.kind {
		case faultUnauthorized:
			g.clearSession()
			g.setState(gatewayStateUnauthorized, fault.reason)
		case faultIdentify:
			g.clearSession()
			g.setState(gatewayStateError, fault.reason)
		default:
			g.setState(gatewayStateError, fault.reason)
		}
		if fault.kind == faultBackoff {
			backoff *= 2
			if backoff > g.backoffMax {
				backoff = g.backoffMax
			}
		}
		if !g.sleep(wait) {
			return
		}
	}
}

// session 是一次连接的全部生命周期：取 token → 取接入点 → 拨号 → Hello →
// Identify/Resume → 心跳 → 读事件。返回的 fault 描述这次为什么断开。
func (g *Gateway) session() (gatewayFault, bool) {
	if g.client == nil || g.endpoint == nil {
		return gatewayFault{kind: faultBackoff, reason: "尚未配置 AppID / AppSecret"}, false
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), g.cfg.SendTimeout())
	defer cancel()
	token, err := g.client.AccessToken(connectCtx)
	if err != nil {
		return faultFromError(err, faultBackoff), false
	}
	endpoint, err := g.endpoint(connectCtx)
	if err != nil {
		return faultFromError(err, faultBackoff), false
	}
	conn, _, err := g.dialer.DialContext(connectCtx, endpoint, nil)
	if err != nil {
		return gatewayFault{kind: faultBackoff, reason: "连不上 QQ 长连接"}, false
	}
	g.setConn(conn)
	defer func() {
		g.clearConn(conn)
		conn.Close()
	}()

	interval, fault, ok := g.readHello(conn)
	if !ok {
		return fault, false
	}

	sessionID, seq := g.sessionState()
	payload := map[string]any{
		"op": gatewayOpIdentify,
		"d": map[string]any{
			"token":   "QQBot " + token,
			"intents": groupIntents,
			"shard":   []int{0, 1},
			"properties": map[string]string{
				"$os": "linux", "$browser": "sforum", "$device": "sforum",
			},
		},
	}
	if sessionID != "" {
		payload = map[string]any{
			"op": gatewayOpResume,
			"d": map[string]any{
				"token": "QQBot " + token, "session_id": sessionID, "seq": seq,
			},
		}
	}
	if err := g.writeJSON(conn, payload); err != nil {
		return gatewayFault{kind: faultBackoff, reason: "长连接写入失败"}, false
	}

	heartbeatDone := make(chan struct{})
	go g.heartbeatLoop(conn, interval, heartbeatDone)
	defer close(heartbeatDone)

	connected := false
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return classifySocketError(err), connected
		}
		var frame gatewayFrame
		if err := json.Unmarshal(message, &frame); err != nil {
			continue
		}
		if frame.S != 0 {
			g.setSeq(frame.S)
		}
		switch frame.Op {
		case gatewayOpDispatch:
			switch {
			case frame.T == "READY":
				g.setSessionID(readSessionID(frame.D))
				g.setState(gatewayStateConnected, "")
				connected = true
			case frame.T == "RESUMED":
				g.setState(gatewayStateConnected, "")
				connected = true
			default:
				g.handleGroupEvent(frame.T, frame.D)
			}
		case gatewayOpReconnect:
			return gatewayFault{kind: faultResume, reason: "平台要求重新连接"}, connected
		case gatewayOpInvalidSession:
			return gatewayFault{kind: faultIdentify, reason: "会话已失效"}, connected
		}
	}
}

// readHello 等到 op=10 Hello，返回心跳间隔。接入点下发的心跳周期就是唯一权威，
// 插件不写死数值。
func (g *Gateway) readHello(conn *websocket.Conn) (time.Duration, gatewayFault, bool) {
	_ = conn.SetReadDeadline(g.now().Add(g.helloTimeout))
	defer conn.SetReadDeadline(time.Time{})
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return 0, classifySocketError(err), false
		}
		var frame gatewayFrame
		if err := json.Unmarshal(message, &frame); err != nil {
			return 0, gatewayFault{kind: faultIdentify, reason: "Hello 帧无法解析"}, false
		}
		if frame.Op != gatewayOpHello {
			continue
		}
		var hello struct {
			HeartbeatInterval int64 `json:"heartbeat_interval"`
		}
		_ = json.Unmarshal(frame.D, &hello)
		interval := time.Duration(hello.HeartbeatInterval) * time.Millisecond
		if interval <= 0 {
			return 0, gatewayFault{kind: faultIdentify, reason: "Hello 未下发心跳周期"}, false
		}
		return interval, gatewayFault{}, true
	}
}

func (g *Gateway) heartbeatLoop(conn *websocket.Conn, interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-g.stopCh:
			return
		case <-ticker.C:
			seq := g.currentSeq()
			payload := map[string]any{"op": gatewayOpHeartbeat, "d": nil}
			if seq != 0 {
				payload["d"] = seq
			}
			if err := g.writeJSON(conn, payload); err != nil {
				return
			}
		}
	}
}

// handleGroupEvent 是群事件的唯一入口：**先**记群 ID（主功能），再按需被动回复。
// 顺序是刻意的——回复要发 HTTP，任何失败都不该拖累群 ID 认领。
func (g *Gateway) handleGroupEvent(eventType string, raw json.RawMessage) {
	if _, ok := claimEvents[eventType]; !ok {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	g.recordClaimPayload(eventType, payload)
	if eventType == "GROUP_AT_MESSAGE_CREATE" {
		g.replyToMention(payload)
	}
}

func (g *Gateway) recordClaim(eventType string, raw json.RawMessage) {
	if _, ok := claimEvents[eventType]; !ok {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	g.recordClaimPayload(eventType, payload)
}

func (g *Gateway) recordClaimPayload(eventType string, payload map[string]any) {
	groupOpenID := groupOpenIDOf(payload)
	if groupOpenID == "" {
		return
	}
	g.state.RecordClaim(groupOpenID, eventType)
	g.log.Infof("qq-bot: 识别到群 %s（%s）", maskGroupID(groupOpenID), eventType)
}

// groupOpenIDOf 兼容平台两种字段写法：group_openid 与 group_id。
func groupOpenIDOf(payload map[string]any) string {
	return firstNonEmpty(
		stringValue(payload, "group_openid"),
		stringValue(payload, "groupOpenid"),
		stringValue(payload, "group_id"),
	)
}

// replyToMention 用事件里的 msg_id 做一次被动回复（不占主动消息频控）。
//
// 被动回复的边界（官方「发送群聊消息」页）：
//   - msg_id 取自 GROUP_AT_MESSAGE_CREATE 事件的 **d.id**，5 分钟内有效；
//   - 每条消息最多被回复 5 次；
//   - msg_id 与 msg_seq 联合去重，相同的 msg_id + msg_seq 重复发送会失败。
//
// 我们的策略是「每群 60 秒最多一次」，所以 msg_seq 固定写 1——同一个 msg_id 只
// 可能被回一次，靠的是下面那道 60 秒闸，而不是自增序号。别把它改成自增：那只会
// 绕开平台的去重保护，把「回过了」变成「再回一次」。
//
// 备注：GROUP_ADD_ROBOT（机器人被拉进群）也能用 event_id 做被动回复，本轮未做。
func (g *Gateway) replyToMention(payload map[string]any) {
	if g == nil || !g.cfg.ReplyOnMention || g.client == nil {
		return
	}
	groupOpenID := groupOpenIDOf(payload)
	msgID := stringValue(payload, "id")
	if groupOpenID == "" || msgID == "" {
		return
	}
	// 不回复机器人自己：群事件的 author 里带 bot 标记。
	if boolValue(nestedValues(payload, "author"), "bot") {
		return
	}
	if !g.mentionGate.allow(groupOpenID, g.now()) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mentionReplyTimeout)
	defer cancel()
	if _, err := g.client.SendPassiveGroupMessage(ctx, groupOpenID, msgID, mentionReplyText, messageFormatText); err != nil {
		// 回复失败只是少一句说明：群 ID 认领已经在前面完成，不能被它拖累。
		g.log.Warnf("qq-bot: 群 %s 的 @ 回复失败：%s", maskGroupID(groupOpenID), adminReason(err))
		return
	}
	g.log.Infof("qq-bot: 已在群 %s 回复一次 @（被动消息）", maskGroupID(groupOpenID))
}

// boolValue 读一个布尔字段；网关载荷是普通 JSON，值就是 bool。
func boolValue(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	flag, ok := values[key].(bool)
	return ok && flag
}

// mentionReplyGate 记录每个群上一次被动回复的时间。进程内、重启即清空——刷屏
// 防护不需要跨重启精确。
type mentionReplyGate struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newMentionReplyGate() *mentionReplyGate {
	return &mentionReplyGate{last: make(map[string]time.Time, 8)}
}

// allow 报告这个群现在能不能回，并在放行时**先占位**：发送失败也不该在 60 秒内
// 重试，那只会把一次故障放大成刷屏。
func (g *mentionReplyGate) allow(groupOpenID string, now time.Time) bool {
	if g == nil || groupOpenID == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, ok := g.last[groupOpenID]; ok && now.Sub(last) < mentionReplyCooldown {
		return false
	}
	// 顺手清掉过期键：群数量有限，但一张永不清的 map 迟早会被问「为什么不清理」。
	for id, at := range g.last {
		if now.Sub(at) >= mentionReplyCooldown {
			delete(g.last, id)
		}
	}
	g.last[groupOpenID] = now
	return true
}

func readSessionID(raw json.RawMessage) string {
	var ready struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &ready); err != nil {
		return ""
	}
	return strings.TrimSpace(ready.SessionID)
}

// classifySocketError 按平台错误码决定下一步：4009/op=7 可 resume，4006/4007 或
// op=9 必须重新 identify，4013/4014 是「平台未授权群事件订阅」，只能长退避重试。
func classifySocketError(err error) gatewayFault {
	var closeError *websocket.CloseError
	if errors.As(err, &closeError) {
		switch closeError.Code {
		case 4009:
			return gatewayFault{kind: faultResume, reason: "连接过期，正在续接"}
		case 4006, 4007:
			return gatewayFault{kind: faultIdentify, reason: "会话无法续接，正在重新鉴权"}
		case 4013, 4014:
			return gatewayFault{kind: faultUnauthorized, reason: "平台未授权群事件订阅"}
		default:
			return gatewayFault{kind: faultBackoff, reason: fmt.Sprintf("长连接被关闭（%d）", closeError.Code)}
		}
	}
	return gatewayFault{kind: faultBackoff, reason: "长连接中断"}
}

func faultFromError(err error, kind string) gatewayFault {
	var qqError *QQError
	if errors.As(err, &qqError) {
		return gatewayFault{kind: kind, reason: qqError.AdminMessage()}
	}
	return gatewayFault{kind: kind, reason: sanitizeReason(err)}
}

func (f gatewayFault) delay(g *Gateway, backoff time.Duration) time.Duration {
	switch f.kind {
	case faultResume, faultIdentify:
		return g.backoffBase
	case faultUnauthorized:
		return g.unauthorizedBackoff
	default:
		return backoff
	}
}

func (g *Gateway) writeJSON(conn *websocket.Conn, payload any) error {
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	return conn.WriteJSON(payload)
}

func (g *Gateway) sleep(duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-g.stopCh:
		return false
	case <-timer.C:
		return true
	}
}

func (g *Gateway) stopped() bool {
	select {
	case <-g.stopCh:
		return true
	default:
		return false
	}
}

func (g *Gateway) setState(state, lastError string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status = state
	if lastError != "" {
		g.lastError = lastError
	}
	if state == gatewayStateConnected {
		g.lastError = ""
		g.connectedAt = g.now()
	}
}

func (g *Gateway) setConn(conn *websocket.Conn) {
	g.mu.Lock()
	g.conn = conn
	g.mu.Unlock()
}

func (g *Gateway) clearConn(conn *websocket.Conn) {
	g.mu.Lock()
	if g.conn == conn {
		g.conn = nil
	}
	g.mu.Unlock()
}

// closeConn 关闭当前连接，让阻塞中的 ReadMessage 立刻返回。
func (g *Gateway) closeConn() {
	g.mu.Lock()
	conn := g.conn
	g.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (g *Gateway) sessionState() (string, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessionID, g.seq
}

func (g *Gateway) setSessionID(sessionID string) {
	if sessionID == "" {
		return
	}
	g.mu.Lock()
	g.sessionID = sessionID
	g.mu.Unlock()
}

func (g *Gateway) clearSession() {
	g.mu.Lock()
	g.sessionID = ""
	g.seq = 0
	g.mu.Unlock()
}

func (g *Gateway) setSeq(seq int64) {
	g.mu.Lock()
	g.seq = seq
	g.mu.Unlock()
}

func (g *Gateway) currentSeq() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.seq
}
