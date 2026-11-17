package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// defaultQQBaseURL 是 QQ 开放平台 OpenAPI 的统一基址（取凭证也在同一域名下）。
	defaultQQBaseURL = "https://api.bot.qq.com"

	tokenPath   = "/app/getAppAccessToken"
	gatewayPath = "/gateway"

	// groupPathPrefix 是群侧 RESTful 前缀：群消息、群信息、群成员都挂在
	// /v2/groups/{group_openid}/… 下。
	groupPathPrefix = "/v2/groups/"

	// tokenRefreshMargin 是提前刷新量：平台在过期前 60 秒内取新凭证可新旧并存，
	// 我们提前 300 秒换新，留足时钟偏差与一次重试的余量。
	tokenRefreshMargin = 300 * time.Second
	// tokenFallbackTTL 是平台没给 expires_in 时的保守有效期。
	tokenFallbackTTL = 7200 * time.Second
)

// QQError 是一次 QQ 接口调用的失败。Reason 是给管理员看的中文文案，Code 只放在
// 括号里供排错；Transient 决定调度器是否值得重试。
type QQError struct {
	Code       int
	HTTPStatus int
	Reason     string
	Transient  bool
	Detail     string
}

func (e *QQError) Error() string {
	if e == nil {
		return ""
	}
	message := e.Reason
	if e.Code != 0 {
		message = fmt.Sprintf("%s(err_code %d)", message, e.Code)
	} else if e.HTTPStatus != 0 {
		message = fmt.Sprintf("%s(HTTP %d)", message, e.HTTPStatus)
	}
	if e.Detail != "" {
		message += "：" + e.Detail
	}
	return message
}

// AdminMessage 是可以直接贴给管理员的一句话，不含请求体与凭证。
func (e *QQError) AdminMessage() string {
	if e == nil {
		return ""
	}
	message := e.Reason
	if e.Code != 0 {
		message += fmt.Sprintf("（err_code %d）", e.Code)
	}
	return message
}

// SendResult 是一次成功投递的回执。
type SendResult struct {
	MessageID string
}

// QQClient 负责取凭证与发群消息。它是并发安全的：token 缓存、刷新序列化由内部锁保证。
type QQClient struct {
	appID     string
	appSecret string
	baseURL   string
	http      *http.Client
	timeout   time.Duration
	log       *logger
	now       func() time.Time

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time

	refreshMu sync.Mutex
}

func newQQClient(config Config, log *logger) *QQClient {
	return &QQClient{
		appID:     config.AppID,
		appSecret: config.AppSecret,
		baseURL:   defaultQQBaseURL,
		http:      &http.Client{},
		timeout:   config.SendTimeout(),
		log:       log,
		now:       time.Now,
	}
}

// configuredBool 报告凭据是否齐全。
func (c *QQClient) configured() bool {
	return c != nil && strings.TrimSpace(c.appID) != "" && strings.TrimSpace(c.appSecret) != ""
}

// AccessToken 返回缓存的访问凭证；临近过期或已过期时重新获取。
func (c *QQClient) AccessToken(ctx context.Context) (string, error) {
	if !c.configured() {
		return "", &QQError{Reason: "尚未配置 AppID / AppSecret"}
	}
	c.mu.Lock()
	cached := c.token
	expiry := c.tokenExpiry
	c.mu.Unlock()
	if cached != "" && c.now().Add(tokenRefreshMargin).Before(expiry) {
		return cached, nil
	}
	return c.refreshToken(ctx)
}

func (c *QQClient) refreshToken(ctx context.Context) (string, error) {
	// 同一个进程里只有一个刷新者；其余调用等它写回缓存后直接命中。
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.mu.Lock()
	cached := c.token
	expiry := c.tokenExpiry
	c.mu.Unlock()
	if cached != "" && c.now().Add(tokenRefreshMargin).Before(expiry) {
		return cached, nil
	}

	body, err := json.Marshal(map[string]string{"appId": c.appID, "clientSecret": c.appSecret})
	if err != nil {
		return "", &QQError{Reason: "构造取凭证请求失败"}
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.baseURL+tokenPath, bytes.NewReader(body))
	if err != nil {
		return "", &QQError{Reason: "构造取凭证请求失败"}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return "", transportError(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", transportError(err)
	}

	var decoded struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Code        int    `json:"code"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		if response.StatusCode >= http.StatusInternalServerError {
			return "", &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
		}
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 取凭证接口返回了无法解析的内容"}
	}
	// 取凭证接口的业务失败仍是 HTTP 200，必须按 body 的 code 判成败。
	if decoded.Code != 0 {
		return "", errorForCode(decoded.Code, decoded.Message, response.StatusCode)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
	}
	if strings.TrimSpace(decoded.AccessToken) == "" {
		return "", &QQError{
			HTTPStatus: response.StatusCode,
			Reason:     "QQ 取凭证接口没有返回访问凭证",
			Detail:     strings.TrimSpace(decoded.Message),
		}
	}

	ttl := tokenFallbackTTL
	if decoded.ExpiresIn > 0 {
		ttl = time.Duration(decoded.ExpiresIn) * time.Second
	}
	c.mu.Lock()
	c.token = decoded.AccessToken
	c.tokenExpiry = c.now().Add(ttl)
	c.mu.Unlock()
	return decoded.AccessToken, nil
}

// invalidateToken 丢弃缓存凭证。HTTP 401 之后调用，强制下一次重新取。
func (c *QQClient) invalidateToken() {
	c.mu.Lock()
	c.token = ""
	c.tokenExpiry = time.Time{}
	c.mu.Unlock()
}

// SendGroupMessage 向一个群发送文本或 Markdown 消息。HTTP 401 表示凭证失效，
// 这里强制重取一次并重试一次；其余失败由 err_code 分类。
func (c *QQClient) SendGroupMessage(ctx context.Context, groupOpenID, content, format string) (SendResult, error) {
	if !c.configured() {
		return SendResult{}, &QQError{Reason: "尚未配置 AppID / AppSecret"}
	}
	token, err := c.AccessToken(ctx)
	if err != nil {
		return SendResult{}, err
	}
	result, err := c.sendOnce(ctx, token, groupOpenID, content, format)
	var qqError *QQError
	if err == nil || !errors.As(err, &qqError) || qqError.HTTPStatus != http.StatusUnauthorized {
		return result, err
	}
	c.invalidateToken()
	fresh, refreshErr := c.AccessToken(ctx)
	if refreshErr != nil {
		return SendResult{}, refreshErr
	}
	return c.sendOnce(ctx, fresh, groupOpenID, content, format)
}

func (c *QQClient) sendOnce(ctx context.Context, token, groupOpenID, content, format string) (SendResult, error) {
	body := map[string]any{"msg_type": 0, "content": content}
	if strings.EqualFold(strings.TrimSpace(format), messageFormatMarkdown) {
		body = map[string]any{"msg_type": 2, "markdown": map[string]string{"content": content}}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return SendResult{}, &QQError{Reason: "构造消息请求失败"}
	}
	endpoint := c.baseURL + "/v2/groups/" + url.PathEscape(groupOpenID) + "/messages"
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return SendResult{}, &QQError{Reason: "构造消息请求失败"}
	}
	request.Header.Set("Authorization", "QQBot "+token)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := c.http.Do(request)
	if err != nil {
		return SendResult{}, transportError(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return SendResult{}, transportError(err)
	}

	var decoded struct {
		ID      string `json:"id"`
		ErrCode int    `json:"err_code"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		if response.StatusCode >= http.StatusInternalServerError {
			return SendResult{}, &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
		}
		return SendResult{}, &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 消息接口返回了无法解析的内容"}
	}
	// 成败只认 err_code：平台在 200 里返回业务失败，也在 4xx 里给结构化错误。
	if decoded.ErrCode != 0 {
		qqError := errorForCode(decoded.ErrCode, decoded.Message, response.StatusCode)
		qqError.Detail = traceDetail(decoded.TraceID)
		return SendResult{}, qqError
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return SendResult{}, &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
	}
	if response.StatusCode == http.StatusUnauthorized {
		return SendResult{}, &QQError{HTTPStatus: response.StatusCode, Reason: "访问凭证已失效"}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return SendResult{}, &QQError{
			HTTPStatus: response.StatusCode,
			Reason:     "QQ 接口拒绝了本次请求",
			Detail:     traceDetail(decoded.TraceID),
		}
	}
	return SendResult{MessageID: decoded.ID}, nil
}

// GroupName 取群名。`GET /v2/groups/{group_openid}/info` 只对**白名单机器人**
// 开放：普通应用会拿到 err_code 11253，所以调用方必须把失败当成「暂时拿不到
// 名字」，而不是致命错误。
func (c *QQClient) GroupName(ctx context.Context, groupOpenID string) (string, error) {
	if !c.configured() {
		return "", &QQError{Reason: "尚未配置 AppID / AppSecret"}
	}
	groupOpenID = strings.TrimSpace(groupOpenID)
	if groupOpenID == "" {
		return "", &QQError{Reason: "群 OpenID 为空"}
	}
	token, err := c.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	name, err := c.groupNameOnce(ctx, token, groupOpenID)
	var qqError *QQError
	if err == nil || !errors.As(err, &qqError) || qqError.HTTPStatus != http.StatusUnauthorized {
		return name, err
	}
	c.invalidateToken()
	fresh, refreshErr := c.AccessToken(ctx)
	if refreshErr != nil {
		return "", refreshErr
	}
	return c.groupNameOnce(ctx, fresh, groupOpenID)
}

func (c *QQClient) groupNameOnce(ctx context.Context, token, groupOpenID string) (string, error) {
	endpoint := c.baseURL + groupPathPrefix + url.PathEscape(groupOpenID) + "/info"
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", &QQError{Reason: "构造群信息请求失败"}
	}
	request.Header.Set("Authorization", "QQBot "+token)
	response, err := c.http.Do(request)
	if err != nil {
		return "", transportError(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", transportError(err)
	}

	// 群侧接口按 err_code 报业务失败，取凭证与接入点那一族用的是 code。两个都读：
	// 万一平台换字段名，「没有权限」也不会被误当成「没有群名」。
	var decoded struct {
		GroupName string `json:"group_name"`
		ErrCode   int    `json:"err_code"`
		Code      int    `json:"code"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		if response.StatusCode >= http.StatusInternalServerError {
			return "", &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
		}
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 群信息接口返回了无法解析的内容"}
	}
	if decoded.ErrCode != 0 {
		return "", errorForCode(decoded.ErrCode, decoded.Message, response.StatusCode)
	}
	if decoded.Code != 0 {
		return "", errorForCode(decoded.Code, decoded.Message, response.StatusCode)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
	}
	if response.StatusCode == http.StatusUnauthorized {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "访问凭证已失效"}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 接口拒绝了本次请求"}
	}
	name := strings.TrimSpace(decoded.GroupName)
	if name == "" {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 群信息接口没有返回群名"}
	}
	return name, nil
}

// GatewayURL 取 WebSocket 接入点。
func (c *QQClient) GatewayURL(ctx context.Context) (string, error) {
	if !c.configured() {
		return "", &QQError{Reason: "尚未配置 AppID / AppSecret"}
	}
	token, err := c.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.baseURL+gatewayPath, nil)
	if err != nil {
		return "", &QQError{Reason: "构造接入点请求失败"}
	}
	request.Header.Set("Authorization", "QQBot "+token)
	response, err := c.http.Do(request)
	if err != nil {
		return "", transportError(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", transportError(err)
	}
	var decoded struct {
		URL     string `json:"url"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 接入点接口返回了无法解析的内容"}
	}
	if decoded.Code != 0 {
		return "", errorForCode(decoded.Code, decoded.Message, response.StatusCode)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "连不上 QQ 接口", Transient: true}
	}
	if strings.TrimSpace(decoded.URL) == "" {
		return "", &QQError{HTTPStatus: response.StatusCode, Reason: "QQ 接入点接口没有返回地址"}
	}
	return strings.TrimSpace(decoded.URL), nil
}

// requestContext 给单次请求套上配置的超时。调用方已有截止时间时尊重更紧的那个。
func (c *QQClient) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok || c.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

// errorForCode 把平台错误码翻译成管理员能读的一句话，并给出是否值得重试。
func errorForCode(code int, message string, httpStatus int) *QQError {
	qqError := &QQError{Code: code, HTTPStatus: httpStatus}
	switch code {
	case 100007, 100016, 10004:
		qqError.Reason = "凭据无效：AppID 或 AppSecret 不正确，或机器人已不存在"
	case 100001:
		qqError.Reason = "取凭据过于频繁，请稍后再试"
		qqError.Transient = true
	case 40034100:
		qqError.Reason = "主动消息已超出频控，请降低推送频率"
		qqError.Transient = true
	case 304022:
		qqError.Reason = "该时段不允许推送"
	case 40034128, 304023:
		qqError.Reason = "主动消息受限：可能群主关闭了「主动推送消息」"
	case 11253:
		qqError.Reason = "该接口需要平台白名单"
	case 40054007, 40054018:
		qqError.Reason = "消息过长，平台拒绝了本次投递"
	default:
		qqError.Reason = "QQ 接口拒绝了本次请求"
		if strings.TrimSpace(message) != "" {
			qqError.Detail = strings.TrimSpace(message)
		}
	}
	return qqError
}

func transportError(err error) *QQError {
	transient := true
	if errors.Is(err, context.Canceled) {
		transient = false
	}
	return &QQError{
		Reason:    "连不上 QQ 接口",
		Transient: transient,
		Detail:    truncateErrorDetail(err),
	}
}

// truncateErrorDetail 只保留传输层错误的一小段文本：完整 URL 与请求体不进日志。
func truncateErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if index := strings.Index(text, "http://"); index > 0 {
		text = text[:index]
	}
	if index := strings.Index(text, "https://"); index > 0 {
		text = text[:index]
	}
	text = strings.TrimSpace(text)
	if len(text) > 160 {
		text = text[:160] + "…"
	}
	return text
}

func traceDetail(traceID string) string {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return ""
	}
	return "trace_id " + traceID
}

// transientError 报告一次失败是否值得重试：网络错误、HTTP 5xx 与平台标记为瞬时的
// 错误码可重试；凭据、权限、长度、主动消息受限等业务错误不重试。
func transientError(err error) bool {
	var qqError *QQError
	if errors.As(err, &qqError) {
		return qqError.Transient
	}
	return false
}

// maskGroupID 只保留群 ID 前 6 位。日志、投递记录、工具输出都走这个函数。
func maskGroupID(groupOpenID string) string {
	runes := []rune(strings.TrimSpace(groupOpenID))
	if len(runes) <= 6 {
		return string(runes)
	}
	return string(runes[:6]) + "…"
}

// messageIDPrefix 是回执里可以对外展示的部分：完整 message id 没有必要出现在
// 管理员文案里，前 8 位足够对照平台日志。
func messageIDPrefix(messageID string) string {
	runes := []rune(strings.TrimSpace(messageID))
	if len(runes) <= 8 {
		return string(runes)
	}
	return string(runes[:8])
}
