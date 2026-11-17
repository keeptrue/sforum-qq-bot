package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestQQClient(t *testing.T, handler http.Handler) *QQClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	config := Config{AppID: "app-id", AppSecret: "app-secret", SendTimeoutSeconds: 2}
	client := newQQClient(config, newLogger(io.Discard, false))
	client.baseURL = server.URL
	client.http = server.Client()
	return client
}

func writeJSON(t *testing.T, writer http.ResponseWriter, status int, payload any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		t.Logf("写测试响应失败：%v", err)
	}
}

func TestQQClientTokenCached(t *testing.T) {
	var tokenCalls atomic.Int32
	var messageCalls atomic.Int32
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case tokenPath:
			tokenCalls.Add(1)
			if request.Method != http.MethodPost {
				t.Errorf("取凭证应是 POST，得到 %s", request.Method)
			}
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-1", "expires_in": 7200})
		case "/v2/groups/GROUP_A/messages":
			messageCalls.Add(1)
			writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg-1"})
		default:
			t.Errorf("意外路径 %s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	}))

	for index := 0; index < 2; index++ {
		result, err := client.SendGroupMessage(t.Context(), "GROUP_A", "hello", messageFormatText)
		if err != nil {
			t.Fatalf("第 %d 次发送失败：%v", index+1, err)
		}
		if result.MessageID != "msg-1" {
			t.Fatalf("回执 id 不对：%q", result.MessageID)
		}
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token 应缓存命中，取凭证次数=%d", tokenCalls.Load())
	}
	if messageCalls.Load() != 2 {
		t.Fatalf("应发送两次，实际 %d", messageCalls.Load())
	}
}

func TestQQClientRefreshesTokenBeforeExpiry(t *testing.T) {
	var tokenCalls atomic.Int32
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == tokenPath {
			count := tokenCalls.Add(1)
			// expires_in 小于 300 秒的提前量：每次调用都应视为「快过期」并刷新。
			writeJSON(t, writer, http.StatusOK, map[string]any{
				"access_token": "tok-" + string(rune('0'+count)), "expires_in": 100,
			})
			return
		}
		writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg"})
	}))
	if _, err := client.AccessToken(t.Context()); err != nil {
		t.Fatalf("第一次取凭证失败：%v", err)
	}
	if _, err := client.AccessToken(t.Context()); err != nil {
		t.Fatalf("第二次取凭证失败：%v", err)
	}
	if tokenCalls.Load() != 2 {
		t.Fatalf("提前刷新未生效，取凭证次数=%d", tokenCalls.Load())
	}
}

func TestQQClientRetriesOnceAfterUnauthorized(t *testing.T) {
	var tokenCalls atomic.Int32
	var messageCalls atomic.Int32
	var secondAuth string
	var mu sync.Mutex
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == tokenPath {
			count := tokenCalls.Add(1)
			token := "tok-1"
			if count > 1 {
				token = "tok-2"
			}
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": token, "expires_in": 7200})
			return
		}
		attempt := messageCalls.Add(1)
		mu.Lock()
		if attempt == 2 {
			secondAuth = request.Header.Get("Authorization")
		}
		mu.Unlock()
		if attempt == 1 {
			writeJSON(t, writer, http.StatusUnauthorized, map[string]any{})
			return
		}
		writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg-2"})
	}))

	result, err := client.SendGroupMessage(t.Context(), "GROUP_A", "hello", messageFormatText)
	if err != nil {
		t.Fatalf("401 后应重取凭证并成功：%v", err)
	}
	if result.MessageID != "msg-2" {
		t.Fatalf("回执不对：%q", result.MessageID)
	}
	if messageCalls.Load() != 2 || tokenCalls.Load() != 2 {
		t.Fatalf("应重试一次并重取凭证，message=%d token=%d", messageCalls.Load(), tokenCalls.Load())
	}
	if secondAuth != "QQBot tok-2" {
		t.Fatalf("重试应使用新凭证，得到 %q", secondAuth)
	}
}

func TestQQClientTextAndMarkdownPayload(t *testing.T) {
	type captured struct {
		MsgType  float64 `json:"msg_type"`
		Content  string  `json:"content"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	var bodies []captured
	var mu sync.Mutex
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == tokenPath {
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok", "expires_in": 7200})
			return
		}
		if authorization := request.Header.Get("Authorization"); authorization != "QQBot tok" {
			t.Errorf("鉴权头不对：%q", authorization)
		}
		if contentType := request.Header.Get("Content-Type"); contentType != "application/json; charset=utf-8" {
			t.Errorf("Content-Type 不对：%q", contentType)
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("读请求体失败：%v", err)
		}
		var body captured
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("请求体不是 JSON：%v", err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg"})
	}))

	if _, err := client.SendGroupMessage(t.Context(), "GROUP A/1", "纯文本", messageFormatText); err != nil {
		t.Fatalf("纯文本发送失败：%v", err)
	}
	if _, err := client.SendGroupMessage(t.Context(), "GROUP A/1", "**Markdown**", messageFormatMarkdown); err != nil {
		t.Fatalf("Markdown 发送失败：%v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("应捕获两个请求体，实际 %d", len(bodies))
	}
	if bodies[0].MsgType != 0 || bodies[0].Content != "纯文本" {
		t.Fatalf("纯文本 payload 不对：%+v", bodies[0])
	}
	if bodies[1].MsgType != 2 || bodies[1].Markdown.Content != "**Markdown**" {
		t.Fatalf("Markdown payload 不对：%+v", bodies[1])
	}
}

func TestQQClientErrorTranslation(t *testing.T) {
	cases := []struct {
		name      string
		code      int
		status    int
		wantPart  string
		transient bool
	}{
		{"凭据无效", 100016, http.StatusOK, "凭据无效", false},
		{"取凭证频繁", 100001, http.StatusOK, "取凭据过于频繁", true},
		{"主动消息频控", 40034100, http.StatusOK, "超出频控", true},
		{"时段限制", 304022, http.StatusOK, "该时段不允许推送", false},
		{"主动消息受限", 40034128, http.StatusOK, "主动消息受限", false},
		{"异步审核", 304023, http.StatusOK, "主动消息受限", false},
		{"白名单接口", 11253, http.StatusOK, "平台白名单", false},
		{"消息过长", 40054007, http.StatusOK, "消息过长", false},
		{"消息异常", 40054018, http.StatusOK, "消息过长", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == tokenPath {
					writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok", "expires_in": 7200})
					return
				}
				writeJSON(t, writer, testCase.status, map[string]any{
					"err_code": testCase.code, "message": "raw platform message", "trace_id": "trace-1",
				})
			}))
			_, err := client.SendGroupMessage(t.Context(), "GROUP_A", "hello", messageFormatText)
			if err == nil {
				t.Fatal("应返回错误")
			}
			message := adminReason(err)
			if !strings.Contains(message, testCase.wantPart) {
				t.Fatalf("翻译文案缺少 %q：%q", testCase.wantPart, message)
			}
			if strings.HasPrefix(message, strconv.Itoa(testCase.code)) {
				t.Fatalf("前台文案不应以裸错误码开头：%q", message)
			}
			if transientError(err) != testCase.transient {
				t.Fatalf("可重试分类不对：want %t got %t", testCase.transient, transientError(err))
			}
		})
	}
}

func TestQQClientTokenBusinessFailure(t *testing.T) {
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 取凭证接口的业务失败仍是 HTTP 200。
		writeJSON(t, writer, http.StatusOK, map[string]any{"code": 100016, "message": "invalid appid or secret"})
	}))
	_, err := client.AccessToken(t.Context())
	if err == nil {
		t.Fatal("取凭证业务失败应返回错误")
	}
	if message := adminReason(err); !strings.Contains(message, "凭据无效") {
		t.Fatalf("应翻译成凭据无效：%q", message)
	}
}

func TestQQClientServerErrorIsTransient(t *testing.T) {
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == tokenPath {
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok", "expires_in": 7200})
			return
		}
		writeJSON(t, writer, http.StatusInternalServerError, map[string]any{})
	}))
	_, err := client.SendGroupMessage(t.Context(), "GROUP_A", "hello", messageFormatText)
	if err == nil {
		t.Fatal("5xx 应返回错误")
	}
	if !transientError(err) {
		t.Fatal("HTTP 5xx 应归类为可重试")
	}
	if message := adminReason(err); !strings.Contains(message, "连不上 QQ 接口") {
		t.Fatalf("5xx 应翻译成连不上：%q", message)
	}
}

func TestQQClientTimeout(t *testing.T) {
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == tokenPath {
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok", "expires_in": 7200})
			return
		}
		time.Sleep(300 * time.Millisecond)
		writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg"})
	}))
	client.timeout = 30 * time.Millisecond
	start := time.Now()
	_, err := client.SendGroupMessage(t.Context(), "GROUP_A", "hello", messageFormatText)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("超时应返回错误")
	}
	if elapsed > time.Second {
		t.Fatalf("超时没有生效，耗时 %v", elapsed)
	}
	if message := adminReason(err); !strings.Contains(message, "连不上 QQ 接口") {
		t.Fatalf("超时应翻译成连不上：%q", message)
	}
}

func TestQQClientGatewayURL(t *testing.T) {
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case tokenPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok", "expires_in": 7200})
		case gatewayPath:
			if authorization := request.Header.Get("Authorization"); authorization != "QQBot tok" {
				t.Errorf("取接入点应带鉴权头，得到 %q", authorization)
			}
			writeJSON(t, writer, http.StatusOK, map[string]any{"url": "wss://api.bot.qq.com/websocket/"})
		default:
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	}))
	endpoint, err := client.GatewayURL(t.Context())
	if err != nil {
		t.Fatalf("取接入点失败：%v", err)
	}
	if endpoint != "wss://api.bot.qq.com/websocket/" {
		t.Fatalf("接入点不对：%q", endpoint)
	}
}

func TestMaskGroupIDAndMessageID(t *testing.T) {
	if got := maskGroupID("ABCDEF123456"); got != "ABCDEF…" {
		t.Fatalf("群 ID 脱敏不对：%q", got)
	}
	if got := maskGroupID("ABC"); got != "ABC" {
		t.Fatalf("短群 ID 不应补省略号：%q", got)
	}
	if got := messageIDPrefix("0123456789ABCDEF"); got != "01234567" {
		t.Fatalf("message id 前缀不对：%q", got)
	}
}

// 被动回复必须带 msg_id 与 msg_seq=1；空 msg_id 直接失败，不发请求。
func TestSendPassiveGroupMessageCarriesMsgID(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case tokenPath:
			writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-1", "expires_in": 7200})
		case "/v2/groups/GROUP_A/messages":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("消息请求体不是 JSON：%v", err)
			}
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
			writeJSON(t, writer, http.StatusOK, map[string]any{"id": "msg-out-1"})
		default:
			t.Errorf("意外路径 %s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
		}
	}))

	result, err := client.SendPassiveGroupMessage(t.Context(), "GROUP_A", "msg-in-1", "hi", messageFormatText)
	if err != nil {
		t.Fatalf("被动回复失败：%v", err)
	}
	if result.MessageID != "msg-out-1" {
		t.Fatalf("回执 id 不对：%q", result.MessageID)
	}

	mu.Lock()
	body := bodies[0]
	count := len(bodies)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("应只发一条，实际 %d 条", count)
	}
	if body["msg_id"] != "msg-in-1" {
		t.Fatalf("请求体缺 msg_id：%v", body)
	}
	if seq, ok := body["msg_seq"].(float64); !ok || int(seq) != 1 {
		t.Fatalf("msg_seq 应固定为 1，得到 %v", body["msg_seq"])
	}
	if body["content"] != "hi" {
		t.Fatalf("正文不对：%v", body["content"])
	}
	if format, ok := body["msg_type"].(float64); !ok || int(format) != 0 {
		t.Fatalf("msg_type 应为 0，得到 %v", body["msg_type"])
	}

	// 空 msg_id：这是调用方的 bug，必须当场失败，绝不退化成本地主动消息。
	if _, err := client.SendPassiveGroupMessage(t.Context(), "GROUP_A", "   ", "hi", messageFormatText); err == nil {
		t.Fatal("空 msg_id 应直接失败")
	}
	mu.Lock()
	count = len(bodies)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("空 msg_id 不该发请求，实际发了 %d 条", count)
	}
}

// 真实线上形状：expires_in 是字符串。写成 int 会让 token 永远拿不到。
func TestAccessTokenAcceptsStringExpiresIn(t *testing.T) {
	var tokenCalls atomic.Int32
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != tokenPath {
			t.Errorf("意外路径 %s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
			return
		}
		tokenCalls.Add(1)
		// 线上实测的逐字形状：expires_in 是字符串。
		writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-string", "expires_in": "7111"})
	}))

	start := client.now()
	token, err := client.AccessToken(t.Context())
	if err != nil {
		t.Fatalf("字符串 expires_in 不该解析失败：%v", err)
	}
	if token != "tok-string" {
		t.Fatalf("token 不对：%q", token)
	}
	client.mu.Lock()
	expiry := client.tokenExpiry
	client.mu.Unlock()
	if want := start.Add(7111 * time.Second); expiry.Sub(want) > time.Second || expiry.Sub(want) < -time.Second {
		t.Fatalf("过期时间应按 7111 秒算，实际 %s（期望约 %s）", expiry, want)
	}

	// 缓存命中：第二次不该再打取凭证接口。
	if _, err := client.AccessToken(t.Context()); err != nil {
		t.Fatalf("第二次取 token 失败：%v", err)
	}
	if calls := tokenCalls.Load(); calls != 1 {
		t.Fatalf("token 应缓存命中，实际取凭证 %d 次", calls)
	}
}

// 数字形状的回归：文档示例与旧版本给的是数字，不能被上一处改动弄坏。
func TestAccessTokenAcceptsNumericExpiresIn(t *testing.T) {
	client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != tokenPath {
			t.Errorf("意外路径 %s", request.URL.Path)
			writeJSON(t, writer, http.StatusNotFound, map[string]any{})
			return
		}
		writeJSON(t, writer, http.StatusOK, map[string]any{"access_token": "tok-number", "expires_in": 7200})
	}))

	start := client.now()
	token, err := client.AccessToken(t.Context())
	if err != nil {
		t.Fatalf("数字 expires_in 不该解析失败：%v", err)
	}
	if token != "tok-number" {
		t.Fatalf("token 不对：%q", token)
	}
	client.mu.Lock()
	expiry := client.tokenExpiry
	client.mu.Unlock()
	if want := start.Add(7200 * time.Second); expiry.Sub(want) > time.Second || expiry.Sub(want) < -time.Second {
		t.Fatalf("过期时间应按 7200 秒算，实际 %s（期望约 %s）", expiry, want)
	}
}

// 错误文案要分清三种失败：不是 JSON、JSON 但字段形状不对、JSON 但没有 access_token。
func TestAccessTokenErrorMessagesAreSpecific(t *testing.T) {
	t.Run("不是 JSON", func(t *testing.T) {
		client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			// 200 但 body 是 HTML：反向代理/网关最典型的伪装形态。
			writer.Header().Set("Content-Type", "text/html")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("<html>bad gateway</html>"))
		}))
		_, err := client.AccessToken(t.Context())
		if err == nil {
			t.Fatal("应返回错误")
		}
		message := adminReason(err)
		if !strings.Contains(message, "不是 JSON") {
			t.Fatalf("应说明不是 JSON：%q", message)
		}
		if !strings.Contains(message, "text/html") {
			t.Fatalf("应带上 Content-Type：%q", message)
		}
		if strings.Contains(message, "bad gateway") {
			t.Fatalf("错误里不该出现 body 原文：%q", message)
		}
	})

	t.Run("JSON 但字段形状不对", func(t *testing.T) {
		client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writeJSON(t, writer, http.StatusOK, map[string]any{
				"access_token": "tok", "expires_in": map[string]any{"nested": 1},
			})
		}))
		_, err := client.AccessToken(t.Context())
		if err == nil {
			t.Fatal("应返回错误")
		}
		message := adminReason(err)
		if !strings.Contains(message, "字段形状不对") {
			t.Fatalf("应说明字段形状不对：%q", message)
		}
		if strings.Contains(message, "tok") {
			t.Fatalf("错误里不该出现 token：%q", message)
		}
	})

	t.Run("JSON 但没有 access_token", func(t *testing.T) {
		client := newTestQQClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writeJSON(t, writer, http.StatusOK, map[string]any{"expires_in": 7200})
		}))
		_, err := client.AccessToken(t.Context())
		if err == nil {
			t.Fatal("应返回错误")
		}
		message := adminReason(err)
		if !strings.Contains(message, "没有返回 access_token") {
			t.Fatalf("应说明缺 access_token：%q", message)
		}
		if !strings.Contains(message, "HTTP 200") {
			t.Fatalf("应带上状态码：%q", message)
		}
	})
}
