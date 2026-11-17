// Package main 是 sforum.qq-bot 的 Protocol V2 后端。
//
// 它把论坛的 topic.created 事件变成 QQ 群里的推送，并提供三个设置页工具
// （测试推送 / 推送状态 / 模板预览）、六条扩展路由（扫码绑定、预览、测试推送、
// 状态、群列表）与一个可选的群 ID 识别长连接。
//
// 边界：论坛数据只走宿主公开契约（Event / HostQuery），插件不直连数据库、
// 不自建业务表；QQ 侧凭据只来自宿主设置项注入的环境变量。
package main

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// 与清单逐字一致的契约常量。改了清单就要同步改这里，manifest_contract_test.go
// 会逐条比对；任何一侧漂移都会让 go test 立刻变红。
//
// 注意 HookId 与 handler 是两个字段：宿主下发的 HookRequest.HookId 是**事件 id**
// （protocol_v2_client.go 的 `HookId: declaration.ID`），handler 只出现在清单里。
const (
	extensionID            = "sforum.qq-bot"
	observeEventID         = "sforum.qq-bot.event.topic-created"
	observeHandlerID       = "sforum.qq-bot.observe.topic-created"
	observeEventName       = "topic.created"
	observeInputSchemaRef  = "sforum.qq-bot.topic-created.input@1"
	observeResultSchemaRef = "sforum.qq-bot.topic-created.result@1"
	toolTestPushID         = "sforum.qq-bot.tool.test_push"
	toolStatusID           = "sforum.qq-bot.tool.status"
	toolPreviewID          = "sforum.qq-bot.tool.preview"
	topicQueryID           = "sforum.core.public_topic.by_id"
	topicQueryPlanVersion  = "1"
)

// topicQueryFunc 是摘要查询的注入点：生产路径是 HostQuery，测试用假实现，
// 避免单测里出现真实 host broker。
type topicQueryFunc func(ctx context.Context, parent *protocolwire.RequestContext, topicID string) (string, error)

// plugin 把全部运行时依赖集中在一处，便于测试逐项替换（sender / query）。
type plugin struct {
	*pluginv2.Server

	cfg   Config
	log   *logger
	state *State

	client     *QQClient
	dispatcher *Dispatcher
	gateway    *Gateway

	// sender 是投递出口。生产路径与 client 是同一个对象；测试注入假实现。
	sender Sender
	// bind / bindStore 是扫码绑定：前者是 q.qq.com 客户端，后者是进程内任务表。
	bind      *BindClient
	bindStore *BindStore

	// groupNames 是群名的尽力而为缓存：群列表接口只在有缓存或还有配额时才问平台。
	groupNames *groupNameCache
	// botProfile 是 GET /users/@me 的缓存：账号状态卡片用它显示当前机器人身份。
	botProfile *botProfileCache

	topicQuery topicQueryFunc
	probeOnce  sync.Once
}

func main() {
	// os.LookupEnv 而不是 os.Getenv：要能区分「键不存在」（用默认值）与
	// 「保存为空串」（空签名就该渲染成空串）。
	pluginv2.Serve(newPlugin(LoadConfigLookup(os.LookupEnv), os.Stderr))
}

// newPlugin 组装运行时。零配置（没有 AppID/AppSecret）时不建立任何外部连接：
// 长连接直接标记 disabled，队列里没有任务，worker 只是空转等待。
func newPlugin(config Config, logOutput io.Writer) *plugin {
	log := newLogger(logOutput, config.LogVerbose)
	// 兜底脱敏：即使将来某条日志不小心带上密钥，也会被替换成 ***。
	log.addSecret(config.AppSecret)
	for _, warning := range config.Warnings {
		log.Warnf("%s", warning)
	}

	state := NewState()
	client := newQQClient(config, log)
	dispatcher := NewDispatcher(config, client, state, log)
	gateway := newGateway(config, client, state, log)

	instance := &plugin{
		cfg:        config,
		log:        log,
		state:      state,
		client:     client,
		dispatcher: dispatcher,
		gateway:    gateway,
		sender:     client,
		bind:       newBindClient(log),
		bindStore:  NewBindStore(time.Now),
		groupNames: newGroupNameCache(time.Now),
		botProfile: newBotProfileCache(time.Now),
	}
	instance.Server = pluginv2.NewServer()
	registry, err := pluginv2.NewAIToolRegistry(aiToolDefinitions(instance)...)
	if err != nil {
		// 声明是静态的：构造失败说明代码与 schema 漂移，宁可不注册工具也不要带病启动。
		log.Errorf("qq-bot: AI 工具声明无效，已跳过注册：%s", err.Error())
	} else {
		instance.Server.WithAIToolRegistry(registry)
	}

	dispatcher.Start()
	gateway.Start()
	return instance
}
