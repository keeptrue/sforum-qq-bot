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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newBindTestClient 把客户端指向 httptest 服务：单测不打真实网络，也不依赖平台。
func newBindTestClient(t *testing.T, handler http.HandlerFunc) *BindClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newBindClient(testLogger())
	client.baseURL = server.URL
	client.connectBaseURL = server.URL
	client.timeout = 2 * time.Second
	return client
}

func writeBindJSON(t *testing.T, writer http.ResponseWriter, payload any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		t.Errorf("写测试响应失败：%v", err)
	}
}

// sealBindSecret 按平台的密文布局造一条密文：IV(12B) ‖ 密文 ‖ tag(16B)。
// 注意 Go 的 Seal 只输出「密文‖tag」，IV 要自己拼在前面。
func sealBindSecret(t *testing.T, keyBase64, plaintext string) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil || len(key) != bindKeyBytes {
		t.Fatalf("测试密钥无效：%v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("构造 AES 失败：%v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("构造 GCM 失败：%v", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("生成 IV 失败：%v", err)
	}
	payload := append(nonce, aead.Seal(nil, nonce, []byte(plaintext), nil)...)
	return base64.StdEncoding.EncodeToString(payload)
}

// 分支一：建任务 + 扫码完成 → 解出凭据。
func TestBindCreateAndPollSuccess(t *testing.T) {
	var issuedKey string
	client := newBindTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case createBindTaskPath:
			var body struct {
				Key string `json:"key"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("建任务请求体不是 JSON：%v", err)
			}
			issuedKey = body.Key
			writeBindJSON(t, writer, map[string]any{
				"retcode": 0, "msg": "", "data": map[string]any{"task_id": "task-1"},
			})
		case pollBindResultPath:
			if issuedKey == "" {
				t.Errorf("轮询发生在建任务之前")
			}
			writeBindJSON(t, writer, map[string]any{
				"retcode": 0, "msg": "", "data": map[string]any{
					"status":             bindRemoteCompleted,
					"bot_appid":          "102030405",
					"bot_encrypt_secret": sealBindSecret(t, issuedKey, "s3cr3t-value"),
				},
			})
		default:
			t.Errorf("未预期的路径：%s", request.URL.Path)
		}
	})

	task, err := client.CreateBindTask(context.Background())
	if err != nil {
		t.Fatalf("建任务失败：%v", err)
	}
	if task.TaskID != "task-1" {
		t.Fatalf("task id 期望 task-1，得到 %q", task.TaskID)
	}
	decodedKey, decodeErr := base64.StdEncoding.DecodeString(task.Key)
	if decodeErr != nil || len(decodedKey) != bindKeyBytes {
		t.Fatalf("密钥应是 base64 编码的 32 字节：len=%d err=%v", len(decodedKey), decodeErr)
	}
	if !strings.Contains(task.ConnectURL, "?task_id=task-1&source=sforum&_wv=2") {
		t.Fatalf("二维码内容不符合协议：%s", task.ConnectURL)
	}

	result, err := client.PollBindTask(context.Background(), task.TaskID, task.Key)
	if err != nil {
		t.Fatalf("轮询失败：%v", err)
	}
	if result.Status != bindStatusScanned {
		t.Fatalf("状态期望 scanned，得到 %q", result.Status)
	}
	if result.AppID != "102030405" || result.AppSecret != "s3cr3t-value" {
		t.Fatalf("凭据解密不对：appID=%q secret=%q", result.AppID, result.AppSecret)
	}
}

// 分支二：还没扫码（status 0 / 1）都是 pending，且不带凭据。
func TestBindPollPending(t *testing.T) {
	for _, remoteStatus := range []int{bindRemoteNone, bindRemotePending} {
		client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
			writeBindJSON(t, writer, map[string]any{
				"retcode": 0, "msg": "", "data": map[string]any{
					"status": remoteStatus, "bot_appid": "", "bot_encrypt_secret": "",
				},
			})
		})
		result, err := client.PollBindTask(context.Background(), "task-1", "unused-key")
		if err != nil {
			t.Fatalf("status %d 不该报错：%v", remoteStatus, err)
		}
		if result.Status != bindStatusPending {
			t.Fatalf("status %d 应映射为 pending，得到 %q", remoteStatus, result.Status)
		}
		if result.AppID != "" || result.AppSecret != "" {
			t.Fatalf("pending 不该带凭据：%q / %q", result.AppID, result.AppSecret)
		}
	}
}

// 分支三：二维码过期。
func TestBindPollExpired(t *testing.T) {
	client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"status": bindRemoteExpired},
		})
	})
	result, err := client.PollBindTask(context.Background(), "task-1", "unused-key")
	if err != nil {
		t.Fatalf("过期不是错误：%v", err)
	}
	if result.Status != bindStatusExpired {
		t.Fatalf("状态期望 expired，得到 %q", result.Status)
	}
}

// 分支四：retcode 非 0 → 中文原因，且不把响应体甩出去。
func TestBindRetcodeFailure(t *testing.T) {
	client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{
			"retcode": 10086, "msg": "bind task expired", "data": map[string]any{"secret": "leak-me"},
		})
	})
	_, err := client.CreateBindTask(context.Background())
	if err == nil {
		t.Fatal("retcode 非 0 必须返回错误")
	}
	var bindError *BindError
	if !errors.As(err, &bindError) {
		t.Fatalf("错误类型期望 *BindError，得到 %T", err)
	}
	if bindError.Retcode != 10086 {
		t.Fatalf("retcode 期望 10086，得到 %d", bindError.Retcode)
	}
	if !strings.Contains(bindError.Reason, "过期") {
		t.Fatalf("msg 应被翻成中文原因，得到 %q", bindError.Reason)
	}
	if message := err.Error(); strings.Contains(message, "leak-me") || strings.Contains(message, "retcode\":10086") {
		t.Fatalf("错误文案泄露了响应体：%s", message)
	}
}

// 分支五：状态已完成但密文坏了 → 可读原因，不放半成品凭据出来。
func TestBindPollBadCiphertext(t *testing.T) {
	key, err := bindRandomKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成测试密钥失败：%v", err)
	}
	cases := map[string]string{
		"随机填充":     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 48)),
		"太短":       base64.StdEncoding.EncodeToString([]byte("short")),
		"非 base64": "!!!not-base64!!!",
	}
	for name, payload := range cases {
		payload := payload
		client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
			writeBindJSON(t, writer, map[string]any{
				"retcode": 0, "msg": "", "data": map[string]any{
					"status": bindRemoteCompleted, "bot_appid": "102030405", "bot_encrypt_secret": payload,
				},
			})
		})
		result, pollErr := client.PollBindTask(context.Background(), "task-1", key)
		if pollErr == nil {
			t.Fatalf("%s：坏密文必须报错，得到 %+v", name, result)
		}
		if !strings.Contains(pollErr.Error(), "解密") {
			t.Fatalf("%s：原因应指向解密失败，得到 %q", name, pollErr.Error())
		}
		if result.AppSecret != "" {
			t.Fatalf("%s：失败时不该返回任何凭据", name)
		}
	}
}

// 分支六：超时 → 「可先手动填凭据」。
func TestBindTimeout(t *testing.T) {
	client := newBindTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-request.Context().Done():
		}
		writeBindJSON(t, writer, map[string]any{"retcode": 0, "msg": "", "data": map[string]any{"task_id": "late"}})
	})
	client.timeout = 30 * time.Millisecond
	_, err := client.CreateBindTask(context.Background())
	if err == nil {
		t.Fatal("超时必须返回错误")
	}
	if !strings.Contains(err.Error(), "超时") || !strings.Contains(err.Error(), "手动") {
		t.Fatalf("超时文案应提示可先手动填凭据，得到 %q", err.Error())
	}
}

// 平台返回未知状态：不猜，直接给可读错误。
func TestBindPollUnknownStatus(t *testing.T) {
	client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"status": 42},
		})
	})
	if _, err := client.PollBindTask(context.Background(), "task-1", "unused-key"); err == nil {
		t.Fatal("未知状态必须报错")
	} else if !strings.Contains(err.Error(), "未知") {
		t.Fatalf("原因应说明状态未知，得到 %q", err.Error())
	}
}

// 建任务没拿到 task_id：拒绝，而不是存一条空任务。
func TestBindCreateWithoutTaskID(t *testing.T) {
	client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{"retcode": 0, "msg": "", "data": map[string]any{}})
	})
	if _, err := client.CreateBindTask(context.Background()); err == nil {
		t.Fatal("缺 task_id 必须报错")
	} else if !strings.Contains(err.Error(), "任务号") {
		t.Fatalf("原因应说明没拿到任务号，得到 %q", err.Error())
	}
}

// 密钥来自 CSPRNG：两次生成不该相同。
func TestBindRandomKeyIsUnique(t *testing.T) {
	first, err := bindRandomKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败：%v", err)
	}
	second, err := bindRandomKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败：%v", err)
	}
	if first == second {
		t.Fatal("两次生成的密钥相同，必须来自 CSPRNG")
	}
}

// 解码器要兼容无填充 / URL 安全写法，不能因为换编码就报「无法解密」。
func TestDecodeBindBase64Variants(t *testing.T) {
	raw := []byte("0123456789abcdef0123456789abcdef")
	variants := []string{
		base64.StdEncoding.EncodeToString(raw),
		base64.RawStdEncoding.EncodeToString(raw),
		base64.URLEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
	}
	for _, encoded := range variants {
		decoded, ok := decodeBindBase64(encoded)
		if !ok || !bytes.Equal(decoded, raw) {
			t.Fatalf("解码失败：%s", encoded)
		}
	}
}

// bindAdminMessage 只吐第一行，不把多行内容带进管理员文案。
func TestBindAdminMessageTrims(t *testing.T) {
	if got := bindAdminMessage(&BindError{Retcode: 7, Reason: "扫码服务暂时不可达"}); got != "扫码服务暂时不可达（retcode 7）" {
		t.Fatalf("文案不对：%q", got)
	}
	if got := bindAdminMessage(errors.New("first line\nsecond line")); got != "first line" {
		t.Fatalf("多行错误应只取首行，得到 %q", got)
	}
	if got := bindAdminMessage(nil); got != "" {
		t.Fatalf("nil 应得到空串，得到 %q", got)
	}
}

// 并发调用同一个客户端不应有数据竞争（配合 -race 更有价值）。
func TestBindClientConcurrentPoll(t *testing.T) {
	key, err := bindRandomKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成测试密钥失败：%v", err)
	}
	client := newBindTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBindJSON(t, writer, map[string]any{
			"retcode": 0, "msg": "", "data": map[string]any{"status": bindRemotePending},
		})
	})
	var waitGroup sync.WaitGroup
	for index := 0; index < 8; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if _, pollErr := client.PollBindTask(context.Background(), "task-1", key); pollErr != nil {
				t.Errorf("并发轮询失败：%v", pollErr)
			}
		}()
	}
	waitGroup.Wait()
}
