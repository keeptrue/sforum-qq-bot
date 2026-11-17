package main

import (
	"context"
	"io"
	"sync"
	"testing"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// testLogger 写 discard，保证测试输出干净；需要观察日志的用例自己建 buffer logger。
func testLogger() *logger { return newLogger(io.Discard, true) }

// 测试里的两个固定调用者：绑定任务属于 testActorUserID，别人（含无 actor）
// 都取不走。取值非零是因为宿主守卫只放行 ActorID>0 的请求。
const (
	testActorUserID int64 = 7
	testOtherUserID int64 = 8
)

// testConfig 是一份「凭据齐全、白名单非空」的基线配置，各用例按需覆盖字段。
func testConfig() Config {
	return Config{
		AppID:                "app-id",
		AppSecret:            "app-secret",
		GroupIDs:             []string{"GROUP_A"},
		MessageFormat:        messageFormatText,
		MessageTemplate:      "【新主题】{{title}}",
		Signature:            defaultSignature,
		ExcerptLimit:         defaultExcerptLimit,
		MaxMessageChars:      defaultMaxMessageChars,
		DailyPushCapPerGroup: defaultDailyPushCapPerGroup,
		MaxPushesPerMinute:   defaultMaxPushesPerMinute,
		MaxRetries:           defaultMaxRetries,
		SendTimeoutSeconds:   defaultSendTimeoutSeconds,
	}
}

type senderCall struct {
	group   string
	content string
	format  string
}

// fakeSender 记录每次投递，并按 behavior 决定成败。
type fakeSender struct {
	mu       sync.Mutex
	calls    []senderCall
	behavior func(index int, group string) (SendResult, error)
}

func (f *fakeSender) SendGroupMessage(_ context.Context, group, content, format string) (SendResult, error) {
	f.mu.Lock()
	index := len(f.calls)
	f.calls = append(f.calls, senderCall{group: group, content: content, format: format})
	behavior := f.behavior
	f.mu.Unlock()
	if behavior == nil {
		return SendResult{MessageID: "msg-1"}, nil
	}
	return behavior(index, group)
}

func (f *fakeSender) Calls() []senderCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]senderCall(nil), f.calls...)
}

// testIdentity 是握手用的稳定制品身份。
func testIdentity() *protocolwire.ExtensionIdentity {
	return &protocolwire.ExtensionIdentity{
		ExtensionId:      extensionID,
		ExtensionVersion: "0.1.0",
		ArtifactDigest:   "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		TrustGrantId:     "grant-test",
		RuntimeEpoch:     1,
		InstanceId:       "instance-test",
	}
}

// handshake 让 *pluginv2.Server 进入已启动状态，之后的 Health 才会通过。
func handshake(t *testing.T, server *pluginv2.Server) *protocolwire.ExtensionIdentity {
	t.Helper()
	identity := testIdentity()
	response, err := server.Handshake(context.Background(), &protocolwire.HandshakeRequest{
		Context:        &protocolwire.RequestContext{RequestId: "handshake", Extension: identity},
		HostProtocols:  []*protocolwire.ProtocolRange{{Protocol: "sforum.plugin", Major: pluginv2.ProtocolMajor}},
		Limits:         &protocolwire.RuntimeLimits{MaxReceiveBytes: 1 << 20, MaxSendBytes: 1 << 20},
		HostApiVersion: pluginv2.HostAPIVersion,
		RuntimeToken:   []byte("0123456789abcdef0123456789abcdef"),
	})
	if err != nil {
		t.Fatalf("握手失败：%v", err)
	}
	if detail := response.GetError(); detail != nil {
		t.Fatalf("握手被拒绝：%s（%s）", detail.GetReason(), detail.GetMessage())
	}
	return identity
}

// newTestHook 组装一个已完成握手、带假 sender 与假查询的插件。
func newTestHook(t *testing.T, config Config, sender Sender, query topicQueryFunc) (*plugin, *protocolwire.ExtensionIdentity) {
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
		topicQuery: query,
	}
	instance.Server = pluginv2.NewServer()
	identity := handshake(t, instance.Server)
	return instance, identity
}

// newHookRequest 构造一次 topic.created 投递。
func newHookRequest(identity *protocolwire.ExtensionIdentity, payload map[string]any, deliveryID string) *pluginwire.HookRequest {
	document, err := pluginv2.NewTypedDocument("sforum.core.topic.created@1", payload)
	if err != nil {
		panic(err)
	}
	return &pluginwire.HookRequest{
		Context: &protocolwire.RequestContext{RequestId: "req-1", Extension: identity},
		// 宿主下发的 HookId 是事件 id，不是 handler。见 main.go 常量注释。
		HookId:     observeEventID,
		HookName:   observeEventName,
		HookKind:   "observe",
		DeliveryId: deliveryID,
		Payload:    document,
	}
}

// fullTopicPayload 是一份「完整载荷」：嵌套对象 + 扁平分类字段。
func fullTopicPayload() map[string]any {
	return map[string]any{
		"topicId":      "12345",
		"categorySlug": "announce",
		"topic": map[string]any{
			"id":    "12345",
			"title": "新版本发布",
			"slug":  "new-release",
			"url":   "https://forum.example.com/t/12345",
		},
		"author":   map[string]any{"displayName": "老后", "username": "backend"},
		"category": map[string]any{"slug": "announce", "name": "公告"},
		"tags":     []any{map[string]any{"name": "发布"}, map[string]any{"name": "版本"}},
	}
}
