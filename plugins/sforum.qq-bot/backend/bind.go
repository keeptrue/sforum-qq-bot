package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 扫码绑定协议来自腾讯官方 npm 包 @tencent-connect/qqbot-connector@1.2.0
// （官方 wiki 未覆盖这两个端点）。平台随时可能调整，因此基址、连接页基址与
// 超时都做成可注入字段，单测走 httptest，不打真实网络。
const (
	defaultBindBaseURL  = "https://q.qq.com"
	createBindTaskPath  = "/lite/create_bind_task"
	pollBindResultPath  = "/lite/poll_bind_result"
	bindConnectPagePath = "/qqbot/openclaw/connect.html"

	// bindConnectSource 是二维码链接里的来源标记，便于平台侧区分调用方。
	bindConnectSource = "sforum"

	// bindKeyBytes 是 AES-256 的密钥长度；平台下发的密钥是它的 base64。
	bindKeyBytes   = 32
	bindNonceBytes = 12
	bindTagBytes   = 16

	defaultBindTimeout   = 10 * time.Second
	bindMaxResponseBytes = 1 << 20
)

// 对外状态与清单 schema 的 enum 逐字一致（pending / scanned / expired）。
const (
	bindStatusPending = "pending"
	bindStatusScanned = "scanned"
	bindStatusExpired = "expired"
)

// 平台侧 data.status 取值：0 未扫码 / 1 已扫码待确认 / 2 已完成 / 3 已过期。
const (
	bindRemoteNone      = 0
	bindRemotePending   = 1
	bindRemoteCompleted = 2
	bindRemoteExpired   = 3
)

// BindError 是一次扫码绑定调用的失败。Reason 是可直接展示给管理员的中文原因，
// 不含密钥、密文或响应体；Retcode 只用于让管理员在平台文档里对号。
type BindError struct {
	Retcode   int
	Reason    string
	Transient bool
}

func (e *BindError) Error() string {
	if e == nil {
		return ""
	}
	if e.Retcode != 0 {
		return fmt.Sprintf("%s（retcode %d）", e.Reason, e.Retcode)
	}
	return e.Reason
}

// bindAdminMessage 把一次失败压成一句管理员可读文案。
func bindAdminMessage(err error) string {
	var bindError *BindError
	if errors.As(err, &bindError) {
		return bindError.Error()
	}
	if err == nil {
		return ""
	}
	// 兜底：只在类型不是 BindError 时可能走到这里，仍然只取首行，避免整段输出。
	text := strings.TrimSpace(err.Error())
	if index := strings.IndexByte(text, '\n'); index > 0 {
		text = text[:index]
	}
	return text
}

// BindTask 是一次新建的绑定任务。Key 是解密密钥的 base64 形态，只在内存里流转：
// 随任务表保存，既不写日志也不进响应体。
type BindTask struct {
	TaskID     string
	Key        string
	ConnectURL string
}

// BindPoll 是一次轮询结果。Scanned 时 AppID / AppSecret 才有值。
type BindPoll struct {
	Status    string
	AppID     string
	AppSecret string
}

// BindClient 是 q.qq.com 扫码绑定客户端。零依赖外部状态，可安全并发使用。
type BindClient struct {
	baseURL        string
	connectBaseURL string
	http           *http.Client
	timeout        time.Duration
	log            *logger
	random         io.Reader
}

func newBindClient(log *logger) *BindClient {
	return &BindClient{
		baseURL:        defaultBindBaseURL,
		connectBaseURL: defaultBindBaseURL,
		http:           &http.Client{},
		timeout:        defaultBindTimeout,
		log:            log,
		random:         rand.Reader,
	}
}

// CreateBindTask 申请一个绑定任务：本端生成 32 字节 CSPRNG 密钥，平台把它绑定到
// 这次二维码上，扫码完成后用它加密下发的凭据。
func (c *BindClient) CreateBindTask(ctx context.Context) (BindTask, error) {
	if c == nil {
		return BindTask{}, &BindError{Reason: "扫码绑定客户端未初始化"}
	}
	key, err := bindRandomKey(c.random)
	if err != nil {
		return BindTask{}, &BindError{Reason: "无法生成绑定密钥"}
	}
	payload, err := json.Marshal(map[string]string{"key": key})
	if err != nil {
		return BindTask{}, &BindError{Reason: "构造绑定请求失败"}
	}
	body, err := c.post(ctx, createBindTaskPath, payload)
	if err != nil {
		return BindTask{}, err
	}
	var data struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return BindTask{}, &BindError{Reason: "扫码服务返回了无法解析的内容"}
	}
	taskID := strings.TrimSpace(data.TaskID)
	if taskID == "" {
		return BindTask{}, &BindError{Reason: "扫码服务没有返回绑定任务号"}
	}
	return BindTask{TaskID: taskID, Key: key, ConnectURL: bindConnectURL(c.connectBaseURL, taskID)}, nil
}

// PollBindTask 查询一次扫码进度。completed 时才解密凭据；解密失败不返回任何
// 半成品数据，避免把坏密文写进设置。
func (c *BindClient) PollBindTask(ctx context.Context, taskID, key string) (BindPoll, error) {
	if c == nil {
		return BindPoll{}, &BindError{Reason: "扫码绑定客户端未初始化"}
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return BindPoll{}, &BindError{Reason: "缺少绑定任务号"}
	}
	payload, err := json.Marshal(map[string]string{"task_id": taskID})
	if err != nil {
		return BindPoll{}, &BindError{Reason: "构造轮询请求失败"}
	}
	body, err := c.post(ctx, pollBindResultPath, payload)
	if err != nil {
		return BindPoll{}, err
	}
	var data struct {
		Status int    `json:"status"`
		AppID  string `json:"bot_appid"`
		Secret string `json:"bot_encrypt_secret"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return BindPoll{}, &BindError{Reason: "扫码服务返回了无法解析的内容"}
	}
	switch data.Status {
	case bindRemoteNone, bindRemotePending:
		return BindPoll{Status: bindStatusPending}, nil
	case bindRemoteExpired:
		return BindPoll{Status: bindStatusExpired}, nil
	case bindRemoteCompleted:
		appID := strings.TrimSpace(data.AppID)
		if appID == "" {
			return BindPoll{}, &BindError{Reason: "扫码服务没有返回 AppID，请重新生成二维码"}
		}
		secret, err := decryptBindSecret(key, data.Secret)
		if err != nil {
			return BindPoll{}, err
		}
		if strings.TrimSpace(secret) == "" {
			return BindPoll{}, &BindError{Reason: "扫码服务返回的 AppSecret 为空，请重新生成二维码"}
		}
		return BindPoll{Status: bindStatusScanned, AppID: appID, AppSecret: secret}, nil
	default:
		return BindPoll{}, &BindError{Reason: "扫码服务返回了未知的绑定状态，请重新生成二维码"}
	}
}

// post 发一次 JSON POST 并返回 data 字段的原始字节。任何失败都只暴露可读原因：
// 响应体、请求体与密钥都不进错误文案。
func (c *BindClient) post(ctx context.Context, path string, payload []byte) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, &BindError{Reason: "构造扫码请求失败"}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		// 传输层错误串里可能带 URL，也可能带请求体摘要，这里只分类不转发。
		c.log.Infof("qq-bot: 扫码绑定请求失败（%s）", path)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
			strings.Contains(err.Error(), "Client.Timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			return nil, &BindError{Reason: "扫码服务响应超时，可先手动填凭据", Transient: true}
		}
		return nil, &BindError{Reason: "扫码服务暂时不可达，可先手动填凭据", Transient: true}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, bindMaxResponseBytes))
	if err != nil {
		return nil, &BindError{Reason: "扫码服务暂时不可达，可先手动填凭据", Transient: true}
	}
	var envelope struct {
		Retcode int             `json:"retcode"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		if response.StatusCode >= http.StatusInternalServerError {
			return nil, &BindError{Reason: "扫码服务暂时不可达，可先手动填凭据", Transient: true}
		}
		return nil, &BindError{Reason: "扫码服务返回了无法解析的内容"}
	}
	if envelope.Retcode != 0 {
		// 业务失败不值得重试：retcode 语义由平台定，重试只会更快撞限流。
		return nil, &BindError{Retcode: envelope.Retcode, Reason: translateBindMessage(envelope.Msg)}
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return nil, &BindError{Reason: "扫码服务暂时不可达，可先手动填凭据", Transient: true}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, &BindError{Reason: "扫码服务拒绝了本次请求"}
	}
	return envelope.Data, nil
}

// requestContext 给单次请求套上超时；调用方已有更紧的截止时间时尊重它。
func (c *BindClient) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok || c.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

// bindConnectURL 拼二维码内容。参数顺序按协议固定，便于平台侧解析与排错。
func bindConnectURL(baseURL, taskID string) string {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBindBaseURL
	}
	return base + bindConnectPagePath +
		"?task_id=" + url.QueryEscape(strings.TrimSpace(taskID)) +
		"&source=" + bindConnectSource +
		"&_wv=2"
}

// translateBindMessage 把平台返回的 msg 翻成中文原因。平台文案可能是中英混排，
// 这里按关键词归类；归不了类的只给一句通用原因，不把原文甩给管理员。
func translateBindMessage(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return "扫码服务拒绝了本次请求"
	case strings.Contains(lower, "expire") || strings.Contains(message, "过期"):
		return "二维码已过期，请重新生成"
	case strings.Contains(lower, "not exist") || strings.Contains(lower, "not found") ||
		strings.Contains(lower, "invalid task") || strings.Contains(message, "不存在"):
		return "绑定任务不存在或已失效，请重新生成二维码"
	case strings.Contains(lower, "frequent") || strings.Contains(lower, "too many") ||
		strings.Contains(lower, "limit") || strings.Contains(message, "频繁"):
		return "扫码服务请求过于频繁，请稍后再试"
	case strings.Contains(lower, "invalid") || strings.Contains(lower, "param") ||
		strings.Contains(lower, "illegal") || strings.Contains(message, "参数"):
		return "扫码服务拒绝了本次请求：参数不被接受"
	default:
		return "扫码服务拒绝了本次请求"
	}
}

// bindRandomKey 生成 base64 编码的 32 字节 CSPRNG 密钥。
func bindRandomKey(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	key := make([]byte, bindKeyBytes)
	if _, err := io.ReadFull(random, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// decryptBindSecret 解出平台下发的 AppSecret。
//
// 布局：IV（12B）‖ 密文 ‖ tag（16B），AES-256-GCM，无附加认证数据。这与 Go 的
// GCM 输出（nonce ‖ 密文 ‖ tag）一致，所以直接把后段当 sealed 交给 Open。
func decryptBindSecret(keyBase64, secretBase64 string) (string, error) {
	key, ok := decodeBindBase64(keyBase64)
	if !ok || len(key) != bindKeyBytes {
		return "", &BindError{Reason: "扫码返回的凭据无法解密，请重新生成二维码后再扫"}
	}
	payload, ok := decodeBindBase64(secretBase64)
	if !ok || len(payload) <= bindNonceBytes+bindTagBytes {
		return "", &BindError{Reason: "扫码返回的凭据无法解密，请重新生成二维码后再扫"}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", &BindError{Reason: "扫码返回的凭据无法解密，请重新生成二维码后再扫"}
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", &BindError{Reason: "扫码返回的凭据无法解密，请重新生成二维码后再扫"}
	}
	if len(payload) < aead.NonceSize()+aead.Overhead() {
		return "", &BindError{Reason: "扫码返回的凭据无法解密，请重新生成二维码后再扫"}
	}
	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", &BindError{Reason: "扫码返回的凭据无法解密，请重新生成二维码后再扫"}
	}
	return string(plain), nil
}

// decodeBindBase64 兼容标准/无填充/URL 安全三种 base64 写法：平台实现换编码时
// 不该表现为「凭据无法解密」。
func decodeBindBase64(value string) ([]byte, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		if decoded, err := encoding.DecodeString(trimmed); err == nil {
			return decoded, true
		}
	}
	return nil, false
}
