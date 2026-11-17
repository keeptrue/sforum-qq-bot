# QQ 官方机器人开放平台 API v2 能力盘点

> 调研对象：QQ 机器人开放平台（`bot.q.qq.com`）API v2
> 调研用途：为 `sforum.qq-bot` 插件的实现决策提供依据（论坛事件 → QQ 推送、QQ 消息 → 论坛回流）
> 调研方式：抓取官方 wiki `sitemap.xml` 全量 URL，本地逐页转文本后逐节核对
> 快照时间：本次抓取时各页「上次更新」最新为 `2026-11-11`（FAQ）与 `2026-10-28`（接口索引）

## 0. 阅读指引

- **权威来源**：<https://bot.q.qq.com/wiki/develop/api-v2/>（API v2 文档根）、<https://bot.q.qq.com/wiki/bot_FAQ/>（常见问题）、<https://bot.q.qq.com/wiki/business/>（运营规范）、管理端 <https://bot.qq.com/>
- **本页原则**：所有能力项均给出官方链接；官方文档没有写的地方一律标注 **「文档未覆盖」**，不臆造接口路径或参数名。
- **路径与域名**：除 `getAppAccessToken` 外，所有 OpenAPI 的统一请求地址为 `https://api.bot.qq.com`（见 [API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html)）。本文表格中的「路径」是相对于该基址的相对路径。
- **鉴权头**：所有 OpenAPI 调用需带 `Authorization: QQBot {ACCESS_TOKEN}`（注意前缀是 `QQBot `，不是 `Bearer`）。
- **扫码绑定（q.qq.com lite）**：不在本 wiki 的 API v2 覆盖范围内，来源与参数见 本文件 §11；该节内容来自腾讯官方 npm SDK，不是官方 wiki 文档。

---

## 1. 鉴权

### 1.1 接入票据

来源：[启动接入](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/getting-started.html)

| 名称 | 描述 | 备注 |
|---|---|---|
| AppID | 机器人 ID | 必须使用 |
| AppSecret | 机器人密钥 | 用于请求签名的密钥 |
| Token（已弃用） | 机器人 Token | 「Token 的鉴权方式已废弃，请使用更安全的 Access Token 鉴权方式。」 |

> AppSecret 同时是 **回调验签密钥**（Ed25519 派生公钥用），见 §2.3。

### 1.2 获取 access_token

来源：[获取访问凭证](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html)

| 项 | 值 |
|---|---|
| HTTP URL | `https://api.bot.qq.com/app/getAppAccessToken` |
| HTTP Method | `POST` |
| Content-Type | `application/json` |

请求参数：

| 名称 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `appId` | string | 是 | 开放平台管理端获得 |
| `clientSecret` | string | 是 | 开放平台管理端获得 |

成功响应：

| 名称 | 类型 | 说明 |
|---|---|---|
| `access_token` | string | 获取到的凭证 |
| `expires_in` | number | 凭证有效时间（秒），「目前是 7200 秒之内的值」 |

失败响应（**HTTP 状态码仍是 200**，必须按 body 里的 `code` 判成败）：

| 名称 | 类型 | 说明 |
|---|---|---|
| `code` | number | 业务错误码 |
| `message` | string | 错误信息，仅供人工排查，「内容可能随时调整」 |

业务错误码（来自同一页）：

| 错误码 | 错误信息 | 排查指南 |
|---|---|---|
| 100001 | Too many requests | 请求过于频繁，请降低调用频率后重试 |
| 100007 | appid invalid | AppID 无效，或机器人状态不正常（被封禁或已删除） |
| 100016 | invalid appid or secret | AppID 或 ClientSecret 不正确 |
| 10004 | 机器人不存在 | AppID 对应的机器人不存在 |

### 1.3 有效期与刷新策略

来源：[获取访问凭证 · 凭证有效期与刷新](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html) 原文：

> 目前 access_token 生命周期默认 7200 秒（2 小时），开发者需要在过期后自行刷新 access_token，保证调用链路权限正常。
> 每次请求不会刷新新的 access_token，在有效期内重复获取会返回相同的值
> 在上一个 access_token 接近过期时间 60 秒内，获取 access_token 时，会获得一个新的 access_token，老的 access_token 在这个 60 秒内仍然有效
> 建议开发者在服务端设置定时刷新凭证的业务逻辑，以防止过期

**结论（对插件的直接含义）**：

- 插件必须自己实现 token 缓存 + 定时刷新（建议在过期前 60~300 秒主动换新），不能每次调用都去取。
- 有效期内重复取 token 返回**同一个值**，所以简单「提前 60 秒刷新」策略是安全的：新旧 token 在 60 秒宽限期内同时有效。
- 同一 token 可能会被多实例共享，插件侧需要单点刷新（宿主内一个进程 + 共享缓存）避免刷新风暴。

### 1.4 频控上限（鉴权相关）

| 接口 | 频控 | 来源 |
|---|---|---|
| `POST /app/getAppAccessToken` | **文档未覆盖**具体 QPS/QPM 数值；仅有错误码 `100001 Too many requests` 提示存在频率限制 | [获取访问凭证](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html) |
| `GET /gateway`（取 WSS 接入点） | `2 QPM / 10 QPM burst` | [获取通用 WSS 接入点](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/gateway.get.html) |
| `GET /gateway/bot`（取带分片 WSS 接入点） | **文档未覆盖** | [获取带分片 WSS 接入点](https://bot.q.qq.com/wiki/develop/api-v2/openapi/wss/shard_url_get.html) |

### 1.5 调用期通用约定

来源：[API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html)

- 成功响应体直接返回业务数据；失败响应体含 `err_code`、`message`、`trace_id`。「请不要依据 message 来判定一个请求是否失败……建议根据 err_code 判断」。
- HTTP 状态码语义：`200` 成功 / `204` 成功无包体 / `201,202` 异步成功（会返回 error body，需特殊处理）/ `401` 认证失败 / `404` 未找到 API / `405` Method 不允许 / `429` 频率限制 / `500,504` 处理失败。
- 链路追踪 ID：HTTP 头 `X-Tps-trace-ID` 或 body 的 `trace_id`，排障时提交给平台。
- **唯一身份机制**：不同 bot(AppID) 拿到的 `user_openid` / `group_openid` / `channel_openid` 互不相同。群聊场景下用户在群内的唯一标识是 `member_openid`。跨 AppID 身份打通（类 unionid）**当前仅支持邮件申请**（见 [FAQ · 如何申请开通机器人Union_openid](https://bot.q.qq.com/wiki/bot_FAQ/)），不能默认可用。

---

## 2. 事件订阅：WebSocket 与 Webhook

两种形态共用同一套 payload 结构与 opcode 体系（[通用数据结构](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/payload.html)）。

### 2.1 两种形态对照

| 维度 | WebSocket（网关长连接） | Webhook（HTTP 回调） |
|---|---|---|
| 官方文档 | [WebSocket 方式](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/websocket.html) | [Webhook 方式](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/webhook.html) |
| 接入地址 | `GET /gateway` 或 `GET /gateway/bot` 动态获取 `wss://api.bot.qq.com/websocket/` | 管理端配置 HTTPS 回调地址，端口仅允许 **80 / 443 / 8080 / 8443** |
| 出网要求 | 插件侧需要能主动建立 WSS 出站连接 | 平台需要能访问插件侧的 HTTPS 回调地址（入站） |
| 鉴权 | 连接后发 `op=2 Identify`，`token` 字段为 `"QQBot {AccessToken}"` | 每个回调请求带 `X-Signature-Ed25519` + `X-Signature-Timestamp` 头，Ed25519 验签 |
| 心跳 | 必须：连接后收到 `op=10 Hello`（含 `heartbeat_interval`），按周期发 `op=1 Heartbeat`，收到 `op=11 Heartbeat ACK` | 无心跳概念 |
| 断线恢复 | `op=6 Resume` + `session_id` + `seq`，服务端补发遗漏事件 | 无状态恢复；平台重投，插件需自己幂等 |
| 事件顺序 | 「尽量有序，但并不保证严格有序」，需要严格有序要自行缓冲并按 `seq` 排序 | 未说明 |
| 多实例/负载均衡 | 原生支持分片（shard），按 `guild_id` 哈希分发 | 每次回调随机落到一个实例，需自行做共享去重/幂等 |
| 回调地址验证 | 不适用 | `op=13`，需用 `plain_token` + `event_ts` 计算 Ed25519 签名回包 |
| 响应要求 | 无（事件在长连接上） | 必须回 `op=12` HTTP Callback ACK |
| 频道发消息前置条件 | **「发送消息接口要求机器人接口需要连接到 websocket 上保持在线状态」** | 同左（若要发频道消息，WS 在线仍是硬前提） |
| 官方是否给出推荐 | **文档未覆盖**：官方未写明「推荐使用哪一种」 | 同左 |

> 插件选型建议（基于上表客观事实，非官方结论）：**双通道**。Webhook 需要公网 HTTPS 回调地址，插件作为 SForum 的子进程通常没有稳定公网入口；WebSocket 是出站连接，天然适合子进程模型。但若插件需要向频道发消息，官方明确要求 WS 在线——因此 WebSocket 应作为**必选**通道，Webhook 仅作可选的「用户自带回调地址」高级选项。

### 2.2 WebSocket 细节

**获取接入点**

- `GET /gateway`（[来源](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/gateway.get.html)）→ `{"url": "wss://api.bot.qq.com/websocket/"}`，频控 `2 QPM / 10 QPM burst`。
- `GET /gateway/bot`（[来源](https://bot.q.qq.com/wiki/develop/api-v2/openapi/wss/shard_url_get.html)）→ 额外返回 `shards`（建议分片数）与 `session_start_limit`（`total` / `remaining` / `reset_after` / `max_concurrency`）。

**握手与鉴权**

```json
// op=2 Identify
{ "op": 2, "d": { "token": "QQBot {AccessToken}", "intents": 513, "shard": [0, 1],
  "properties": { "$os": "linux", "$browser": "my_library", "$device": "my_library" } } }
```

- `token` 格式为 `"QQBot {AccessToken}"`。
- `shard`：无需分片用 `[0, 1]`；`[0,4]` 表示共 4 片、本连接是第 0 片，需另建 `[1,4] [2,4] [3,4]` 才能收全事件。
- 分片哈希规则：`shard_id = (guild_id >> 22) % num_shards`（同一频道固定从同一连接推送）。
- 鉴权成功后收到 `op=0 / t=READY` 事件，含 `session_id`。

**心跳与恢复**

```json
{ "op": 1, "d": 251 }      // 客户端心跳，d = 收到的最新 s，首次连接传 null
{ "op": 11 }               // 服务端 Heartbeat ACK
{ "op": 6, "d": { "token": "...", "session_id": "...", "seq": 1337 } }  // Resume
{ "op": 0, "s": 2002, "t": "RESUMED", "d": "" }                          // 补发完成
```

**Opcode 全表**（[通用数据结构](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/payload.html)）

| CODE | 名称 | 接入方式 | 客户端行为 | 描述 |
|---|---|---|---|---|
| 0 | Dispatch | webhook/websocket | Receive | 服务端进行消息推送 |
| 1 | Heartbeat | websocket | Send/Receive | 客户端或服务端发送心跳 |
| 2 | Identify | websocket | Send | 客户端发送鉴权 |
| 6 | Resume | websocket | Send | 客户端恢复连接 |
| 7 | Reconnect | websocket | Receive | 服务端通知客户端重新连接 |
| 9 | Invalid Session | websocket | Receive | identify/resume 参数有错时返回 |
| 10 | Hello | websocket | Receive | 建连后网关下发的第一条消息 |
| 11 | Heartbeat ACK | websocket | Receive/Reply | 心跳成功回执 |
| 12 | HTTP Callback ACK | webhook | Reply | HTTP 回调模式回包 |
| 13 | 回调地址验证 | webhook | Receive | 开放平台对机器人服务端进行验证 |

**WebSocket 错误码与处置**（[来源](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/error-trace/websocket.html)）

| 值 | 含义 | 可重试 RESUME | 可重试 IDENTIFY |
|---|---|---|---|
| 4001 | 无效的 opcode | 否 | 否 |
| 4002 | 无效的 payload | 否 | 否 |
| 4007 | seq 错误 | 否 | 是 |
| 4006 | 无效的 session id，无法继续 resume | 否 | 是 |
| 4008 | 发送 payload 过快，请重新连接并遵守频控 | 是 | 是 |
| 4009 | 连接过期，请重连并执行 resume | 是 | 是 |
| 4010 | 无效的 shard | 否 | 否 |
| 4011 | 连接需要处理的 guild 过多 | 否 | 否 |
| 4012 | 无效的 version | 否 | 否 |
| 4013 | 无效的 intent | 否 | 否 |
| 4014 | intent 无权限 | 否 | 否 |
| 4900~4913 | 内部错误，请重连 | 否 | 是 |
| 4914 | **机器人已下架，只允许连接沙箱环境**，请断开连接并检验当前连接环境 | 否 | 否 |
| 4915 | 机器人已封禁，不允许连接 | 否 | 否 |

官方处置建议原文：「4009 可以重新发起 resume；4914，4915 不可以连接，请联系官方解封；其他错误，请重新发起 identify。」

### 2.3 Webhook 细节

来源：[Webhook 方式](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/webhook.html)

**回调地址验证（op=13）**

平台请求：

```json
{ "d": { "plain_token": "Arq0D5A61EgUu4OxUvOp", "event_ts": "1725442341" }, "op": 13 }
```

机器人必须返回：

```json
{ "plain_token": "Arq0D5A61EgUu4OxUvOp", "signature": "87befc99c42c651b3aac0278e71ada338433ae26fcb24307bdc5ad38c1adc2d01bcfcadc0842edac85e85205028a1132afe09280305f13aa6909ffc2d652c706" }
```

计算方式：把 `botSecret` 反复 `strings.Repeat(seed, 2)` 直到长度 ≥ 32，截取前 32 字节作为 Ed25519 seed → `ed25519.GenerateKey` 得私钥 → 对 `event_ts + plain_token` 签名 → hex 编码。

**事件回调验签（Ed25519）**

来源：[安全和授权](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/sign.html)

| 字段 | 说明 |
|---|---|
| `X-Signature-Ed25519` | HTTP Header 中的签名（64 字节，hex） |
| `X-Signature-Timestamp` | 签名时间戳 |
| HTTP Body | 原始请求体（**按字节原样参与验签，不要先解析再序列化**） |

验签流程（官方 Go 示例）：

1. 用 `botSecret` 重复拼接得到 32 字节 seed，`ed25519.GenerateKey(rand)` 得到公钥/私钥（验签只需公钥）。
2. `hex.DecodeString(X-Signature-Ed25519)` 得到 64 字节 signature。
3. 拼接签名体：`msg = timestamp + body`（**顺序固定**）。
4. `ed25519.Verify(publicKey, msg.Bytes(), sig)`。

**注意**：Webhook 回调地址验证用 **私钥签名**（平台验签），事件回调验签用 **公钥验签**（平台签名），两者公钥由同一 seed 派生。

**响应要求**：事件回调应回 `op=12` HTTP Callback ACK（见 opcode 表）。FAQ 另提示：「遇到 webhook 数据突然中断，可优先排查防火墙等网络安全设置」（[FAQ](https://bot.q.qq.com/wiki/bot_FAQ/)）。

### 2.4 事件订阅 Intents（位掩码）

来源：[通用数据结构 · 事件订阅 Intents](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/payload.html)

| Intent | 位 | 事件 |
|---|---|---|
| GUILDS | `1<<0` | GUILD_CREATE / GUILD_UPDATE / GUILD_DELETE / CHANNEL_CREATE / CHANNEL_UPDATE / CHANNEL_DELETE |
| GUILD_MEMBERS | `1<<1` | GUILD_MEMBER_ADD / GUILD_MEMBER_UPDATE / GUILD_MEMBER_REMOVE |
| GUILD_MESSAGES | `1<<9` | MESSAGE_CREATE / MESSAGE_DELETE —— **仅「私域」机器人可设** |
| GUILD_MESSAGE_REACTIONS | `1<<10` | MESSAGE_REACTION_ADD / MESSAGE_REACTION_REMOVE |
| DIRECT_MESSAGE | `1<<12` | DIRECT_MESSAGE_CREATE / DIRECT_MESSAGE_DELETE |
| **GROUP_AND_C2C_EVENT** | `1<<25` | C2C_MESSAGE_CREATE / FRIEND_ADD / FRIEND_DEL / C2C_MSG_REJECT / C2C_MSG_RECEIVE / **GROUP_AT_MESSAGE_CREATE** / GROUP_ADD_ROBOT / GROUP_DEL_ROBOT / GROUP_MSG_REJECT / GROUP_MSG_RECEIVE |
| INTERACTION | `1<<26` | INTERACTION_CREATE |
| MESSAGE_AUDIT | `1<<27` | MESSAGE_AUDIT_PASS / MESSAGE_AUDIT_REJECT |
| FORUMS_EVENT | `1<<28` | FORUM_THREAD_CREATE/UPDATE/DELETE、FORUM_POST_CREATE/DELETE、FORUM_REPLY_CREATE/DELETE —— **仅「私域」机器人可设** |
| AUDIO_ACTION | `1<<29` | AUDIO_START / AUDIO_FINISH / AUDIO_ON_MIC / AUDIO_OFF_MIC |
| PUBLIC_GUILD_MESSAGES | `1<<30` | AT_MESSAGE_CREATE / PUBLIC_MESSAGE_DELETE |

- 计算示例：`0 | 1<<30 | 1<<1` 表示订阅 `PUBLIC_GUILD_MESSAGES` + `GUILD_MEMBERS`。
- **权限**：「除了 GUILDS，PUBLIC_GUILD_MESSAGES，GUILD_MEMBERS 事件是基础的事件，默认有权限订阅之外，其他的特殊事件，都需要经过申请才能够使用，如果在鉴权的时候传递了无权限的 intents，websocket 会报错，并直接关闭连接。」
- 权限被取消时：当前连接不报错但收不到事件；重新连接会报错。

> 插件含义：**群聊与单聊的事件都在 `GROUP_AND_C2C_EVENT (1<<25)` 一个 intent 里**，无需分别订阅。频道 @ 消息需要 `1<<30`。

---

## 3. 消息能力清单

### 3.1 发送 / 撤回 / 上传（OpenAPI）

统一基址 `https://api.bot.qq.com`，统一鉴权头 `Authorization: QQBot {ACCESS_TOKEN}`。

| 能力 | 方法与路径 | 必填参数 | 频控 | 官方链接 |
|---|---|---|---|---|
| 发送群聊消息 | `POST /v2/groups/{group_openid}/messages` | 路径 `group_openid`；body 至少给 `msg_type` + 对应内容字段 | 100 QPS | [发送群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html) |
| 撤回群聊消息 | `DELETE /v2/groups/{group_openid}/messages/{message_id}` | `group_openid`、`message_id` | 10 QPS | [撤回群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages_message_id.delete.html) |
| 发送单聊消息 | `POST /v2/users/{user_openid}/messages` | `user_openid`；body 同上 | 100 QPS | [发送单聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_messages.post.html) |
| 撤回单聊消息 | `DELETE /v2/users/{user_openid}/messages/{message_id}` | `user_openid`、`message_id` | 10 QPS | [撤回单聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_messages_message_id.delete.html) |
| 流式发送单聊消息 | `POST /v2/users/{user_openid}/stream_messages` | `user_openid`；body `index`、`content_raw`、`stream_msg_id`（续片） | 50 QPS | [流式发送单聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_stream_messages.post.html) |
| 群聊富媒体上传 | `POST /v2/groups/{group_openid}/files` | `group_openid`；body `file_type`（1图片/2视频/3语音/4文件），上传方式三选一 | 50 QPS | [群聊富媒体上传](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_files.post.html) |
| 单聊富媒体上传 | `POST /v2/users/{user_openid}/files` | `user_openid`；同上 | 50 QPS | [单聊富媒体上传](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_files.post.html) |
| 群聊富媒体预上传 | `POST /v2/groups/{group_id}/upload_prepare` | `group_id`、`file_type`、`file_size`、`file_name`、`md5`、`sha1`、`md5_10m` | 10 QPS | [群聊富媒体预上传](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_id_upload_prepare.post.html) |
| 群聊分片上传完成 | `POST /v2/groups/{group_id}/upload_part_finish` | `group_id`、`upload_id`、`part_index`、`block_size`、`md5` | 10 QPS | [群聊分片上传完成](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_id_upload_part_finish.post.html) |
| 单聊富媒体预上传 | `POST /v2/users/{user_id}/upload_prepare` | `user_id` + 同群聊 | 10 QPS | [单聊富媒体预上传](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_id_upload_prepare.post.html) |
| 单聊分片上传完成 | `POST /v2/users/{user_id}/upload_part_finish` | `user_id` + 同群聊 | 10 QPS | [单聊分片上传完成](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_id_upload_part_finish.post.html) |
| Markdown 素材上传（申请） | `POST /v2/media-images/prepare` | `file_name`、`file_size`、`content_type`、`md5`、`sha1` | **文档未覆盖** | [素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html) |
| Markdown 素材上传（直传） | `PUT <presigned_url>` | 预签名 URL + 与 prepare 声明一致的 `Content-Type` | — | [素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html) |
| Markdown 素材上传（完成） | `POST /v2/media-images/complete` | `upload_id`、`md5`（= COS 返回的 ETag） | **文档未覆盖** | [素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html) |
| 查询有效素材 | `GET /v2/media-images?cursor=` | 可选 `cursor` | **文档未覆盖** | [素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html) |
| 删除素材 | `DELETE /v2/media-images/{image_id}/delete` | `image_id` | **文档未覆盖** | [素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html) |
| 发送子频道消息 | `POST /channels/{channel_id}/messages` | `channel_id`；body 至少 `content`/`embed`/`ark`/`image`/`file_image`/`markdown` 之一 | **文档未覆盖**（限制见 §5） | [发送子频道消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/send.html) |
| 撤回子频道消息 | `DELETE /channels/{channel_id}/messages/{message_id}?hidetip=false` | `channel_id`、`message_id` | **文档未覆盖** | [撤回子频道消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/recall.html) |
| 创建频道私信会话 | `POST /users/@me/dms` | `recipient_id`、`source_guild_id` | **文档未覆盖** | [频道私信](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/dms.html) |
| 机器人发表情表态 | `PUT /channels/{channel_id}/messages/{message_id}/reactions/{type}/{id}` | `channel_id`、`message_id`、`type`、`id` | **文档未覆盖** | [表情表态](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/trans/emoji.html) |
| 删除表情表态 | `DELETE /channels/{channel_id}/messages/{message_id}/reactions/{type}/{id}` | 同上 | **文档未覆盖** | [表情表态](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/trans/emoji.html) |
| 拉取表态用户列表 | `GET /channels/{channel_id}/messages/{message_id}/reactions/{type}/{id}?cookie=&limit=` | 同上 + 可选 `cookie`、`limit`（默认 20，最多 50） | **文档未覆盖** | [表情表态](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/trans/emoji.html) |
| 互动事件响应 | `PUT /interactions/{interaction_id}` | `interaction_id`（来自 `INTERACTION_CREATE` 的 `d.id`，不带 `INTERACTION_CREATE:` 前缀）；body `code`（0=成功） | 50 QPS | [互动事件响应](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/interactions_interaction_id.put.html) |
| 获取机器人详情 | `GET /users/@me` | — | 50 QPS | [获取机器人详情](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/users_me.get.html) |
| 获取机器人频道列表 | `GET /users/@me/guilds` | 可选 `before`（分页） | 50 QPS | [获取机器人频道列表](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/users_me_guilds.get.html) |
| 获取群基本信息 | `GET /v2/groups/{group_openid}/info` | `group_openid` | 30 QPM | [获取群基本信息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_info.get.html) |
| 获取机器人群内状态 | `GET /v2/groups/{group_openid}/bot_state` | `group_openid` | 30 QPM | [获取机器人群内状态](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_bot_state.get.html) |
| 获取群成员列表 | `GET /v2/groups/{group_openid}/members?cursor=` | `group_openid` | 60 QPM | [获取群成员列表](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_members.get.html) |
| 生成分享链接 | `POST /v2/generate_url_link` | 可选 `callback_data`（≤32 字符，用户加好友时透传） | 50 QPS | [生成分享链接](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_generate_url_link.post.html) |
| 查询/修改全局自定义菜单 | `GET /v2/menu` / `PUT /v2/menu` | `PUT` 传 `menu`（覆盖式） | 30 QPM / 5 QPM | [查询全局自定义菜单](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_menu.get.html) / [修改全局自定义菜单](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_menu.put.html) |
| 指令面板 CRUD | `GET/POST /v2/panels`、`GET/PUT/DELETE /v2/panels/{panel_id}`、`PUT /v2/panels/{panel_id}/target` | `scope`（c2c/group/channel/dm）为创建必填 | 30 QPM / 10 QPM | [查询指令面板列表](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_panels.get.html) / [创建指令面板](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_panels.post.html) |

> ⚠️ 上表 `GET /v2/groups/{group_openid}/info`、`/bot_state`、`/members` 三个接口的错误码里都有 `11253 应用无接口访问权限 —— 该接口仅白名单机器人可用，请联系平台运营申请权限`。`/members` 页还标注「该能力正在内邀接入中，敬请期待」。

完整接口索引：[接口文档索引](https://bot.q.qq.com/wiki/develop/api-v2/autogen/)。

### 3.2 群聊 / 单聊发消息的请求体字段

来源：[发送群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html)、[发送单聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_messages.post.html)

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `msg_type` | integer | 否（但实际必填） | 0=纯文本(`content`) / 2=Markdown(`markdown`) / 6=输入中状态（仅单聊） / 7=富媒体(`media`) / 8=图文卡片(`card`) |
| `content` | string | `msg_type=0` 时必填 | 纯文本；「传了 markdown 后此字段必须为空」 |
| `markdown` | object | `msg_type=2` 时必填 | `{ "content": "..." }`，或 `{ "custom_template_id": "...", "params": [...] }` |
| `keyboard` | object | 否 | 短形式 `{ "id": "..." }`；长形式 `{ "content": { "rows": [...] } }` |
| `msg_id` | string | 否 | 被动回复用。从事件 `d.id` 取 |
| `event_id` | string | 否 | 被动回复用，与 `msg_id` 二选一。群侧支持 `INTERACTION_CREATE`/`GROUP_ADD_ROBOT`/`GROUP_MSG_RECEIVE`；单聊侧支持 `INTERACTION_CREATE`/`C2C_MSG_RECEIVE`/`FRIEND_ADD` |
| `msg_seq` | integer | 否 | 回复序号，默认 1；「相同的 msg_id + msg_seq 重复发送会失败」，需递增 |
| `media` | object | `msg_type=7` 时必填 | `{ "file_info": "..." }` |
| `message_reference` | object | 否 | 引用回复 `{ "message_id": "REFIDX_..." }` |
| `card` | object | `msg_type=8` 时必填 | `{ "type": "tuwen", "content": { title, description, url, pic_url } }` |
| `is_wakeup` | boolean | 否 | **仅单聊**：互动召回消息，与 `msg_id`/`event_id` 互斥 |
| `input_notify` | object | `msg_type=6` 时必填 | `{ "input_type": 1, "input_second": 60 }`（最长 60s） |

### 3.3 接收事件

| 事件 | Intent | 触发条件 | 官方链接 |
|---|---|---|---|
| `GROUP_AT_MESSAGE_CREATE` | GROUP_AND_C2C_EVENT | 用户在群里 @机器人 | [群@机器人消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_at_message_create.html) |
| `GROUP_MESSAGE_CREATE` | GROUP_AND_C2C_EVENT | 机器人开启「接收所有消息」后群内每条消息 | [群消息（全量模式）](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_message_create.html) |
| `C2C_MESSAGE_CREATE` | GROUP_AND_C2C_EVENT | 用户单聊发消息 | [单聊消息事件](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/c2c_message_create.html) |
| `INTERACTION_CREATE` | INTERACTION | 消息按钮点击、快捷菜单、反馈、清空会话等 | [互动事件](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/interaction_create.html) |
| `GROUP_ADD_ROBOT` / `GROUP_DEL_ROBOT` | GROUP_AND_C2C_EVENT | 机器人被加/移出群 | [接口文档索引](https://bot.q.qq.com/wiki/develop/api-v2/autogen/) |
| `GROUP_MEMBER_ADD` / `GROUP_MEMBER_REMOVE` | GROUP_AND_C2C_EVENT | 群成员进出 | [接口文档索引](https://bot.q.qq.com/wiki/develop/api-v2/autogen/) |
| `GROUP_MSG_RECEIVE` / `GROUP_MSG_REJECT` | GROUP_AND_C2C_EVENT | 群管理员开启/关闭「主动消息」推送 | [接口文档索引](https://bot.q.qq.com/wiki/develop/api-v2/autogen/) |
| `C2C_MSG_RECEIVE` / `C2C_MSG_REJECT` | GROUP_AND_C2C_EVENT | 用户在资料卡开启/关闭主动消息 | [接口文档索引](https://bot.q.qq.com/wiki/develop/api-v2/autogen/) |
| `AT_MESSAGE_CREATE` | PUBLIC_GUILD_MESSAGES | 频道内 @机器人 或 回复机器人 | [频道消息事件](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/event.html) |
| `MESSAGE_CREATE` | PUBLIC_GUILD_MESSAGES（私域） | 频道内全部消息 | [频道消息事件](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/event.html) |
| `DIRECT_MESSAGE_CREATE` | DIRECT_MESSAGE | 频道私信 | [频道消息事件](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/event.html) |
| `MESSAGE_REACTION_ADD` / `MESSAGE_REACTION_REMOVE` | GUILD_MESSAGE_REACTIONS | 频道消息被表态/取消 | [表情表态](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/trans/emoji.html) |
| `MESSAGE_AUDIT_PASS` / `MESSAGE_AUDIT_REJECT` | MESSAGE_AUDIT | 频道主动消息审核结果 | [频道消息事件](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/event.html) |

**去重要点（官方明确要求）**：

- `GROUP_AT_MESSAGE_CREATE` / `C2C_MESSAGE_CREATE`：「相同 msg_id 可能多次推送，开发者需结合 msg_seq 做去重」；C2C 事件补充「需结合 `message_scene.ext` 中的 `msg_idx` 做去重」。
- 事件体核心字段：`id`、`author`（含 `user_openid` / `member_openid` / `member_role`）、`content`、`timestamp`、`message_type`、`message_scene`（`source` + `ext[]`，ext 内含 `msg_idx=` / `ref_msg_idx=` / `auth_token=`）、`attachments[]`、`mentions[]`、`ark_data`、`msg_elements[]`。

### 3.4 消息类型支持矩阵

来源：[消息类型](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/type/overview.html)

| 类型 | 单聊 | 群聊 | 频道 |
|---|---|---|---|
| 文本 | 收发 ✅ | 收发 ✅ | 收发 ✅ |
| Markdown | 发 ✅ / 收 ❌ | 收发 ✅ | 发 ✅ / 收 ❌ |
| 图片 / 视频 / 语音 | 收发 ✅ | 收发 ✅ | 收发 ✅ |
| 文件 | 收发 ✅ | 收发 ✅ | ❌ |
| 结构化卡片（ARK） | 发 ❌ / 收 ✅ | 发 ❌ / 收 ✅ | 发 ❌ / 收 ❌ |
| Embed | ❌ | ❌ | 发 ✅ / 收 ❌ |
| 表情表态 | ❌ | ❌ | 收发 ✅ |
| 引用消息 | 收 ✅ | 收 ✅ | 收 ✅ |

发送侧官方写的是「只有 `msg_type=0/2/3/7` 四种」，但同页上方的表格只列了 `0 / 2 / 7`，而 [发送群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html) 的请求体说明写的是 `0=纯文本 2=Markdown 7=富媒体 8=图文卡片`。**图文卡片的 `msg_type` 到底是 3 还是 8，官方两处表述冲突，见 §8 第 18 条**；本文其余表格按接口页采用 `8`。

### 3.5 富媒体与素材

来源：[富媒体消息概述](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/rich-media.html)、[素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html)

| file_type | 类型 | 格式 | 软限制 | 硬限制 |
|---|---|---|---|---|
| 1 | 图片 | png / jpg（另支持 gif/webp/bmp） | 20 MB | 200 MB |
| 2 | 视频 | mp4 | 30 MB | 200 MB |
| 3 | 语音 | silk（另支持 mp3/wav/ogg） | 20 MB | 200 MB |
| 4 | 文件 | 任意 | 200 MB | 200 MB |

- 超软限制降级为文件类型，超硬限制报错。
- 上传三选一：URL 上传 / 分片上传合并（**官方推荐**）/ 二进制直传（≤ 5 MB）。
- 分片上传四步：`upload_prepare` → 逐片 HTTP PUT 到 `presigned_url` → `upload_part_finish` → 带 `upload_id` 调 `files` 接口合并得 `file_info`。分片大小默认 5 MB，`upload_config` 下发 `concurrency` / `retry_timeout` / `retry_delay`。
- `file_info` **有 ttl**，过期需重新上传；单聊与群聊上传接口互相隔离，文件不能跨场景使用。
- Markdown 图片素材：单图 ≤ 10 MB，png/jpeg/gif/webp，单机器人 100 张上限，滑动过期（最后一次引用起 90 天），Markdown 中以 `qqbot-media://image/v1/<token>` 引用；「qqbot-media:// 链接与机器人绑定，其他机器人无法引用」。

### 3.6 @ 提及

| 场景 | 语法 | 来源 |
|---|---|---|
| 频道 | `<@user_id>` 或 `<@!user_id>` 解析为 @用户；`@everyone` 需机器人有发 @所有人 权限；`<#channel_id>` 跳子频道；`<emoji:id>` 系统表情 | [内嵌格式](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/format.html) |
| 群聊（接收侧） | `@某成员` 在 `content` 中被替换为 `<@{member_openid}>`；`@全体成员` 替换为 `<@all>`；被 @ 成员列表见 `mentions` | [群消息（全量模式）](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_message_create.html) |
| 群聊（发送侧） | **文档未覆盖**：群聊发消息接口的请求体没有 mentions 字段，官方也未给出群聊 @ 成员的发送语法 | — |
| @全体成员 配额 | 错误码 `304019 AT_EVERYONE_TIMES @全体成员 次数超限`（数量未在文档中给出） | [API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html) |

### 3.7 消息按钮（内联键盘）

来源：[消息按钮](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/trans/msg-btn.html)

- 两种形态：`{ "keyboard": { "id": "123" } }`（**需申请模版**）与 `{ "keyboard": { "content": { "rows": [...] } } }`（**内邀开通**）。
- 自定义按钮最多 5 行 × 每行 5 个按钮。
- 按钮字段：`id`、`render_data.label` / `visited_label` / `style`（0 灰色线框 / 1 蓝色线框）、`action.type`（0 跳转 / 1 回调后台 / 2 指令按钮）、`action.permission.type`（0 指定用户 / 1 仅管理者 / 2 所有人 / 3 指定身份组，仅频道）、`action.data`、`action.unsupport_tips`、`action.reply` / `enter` / `anchor`（客户端版本 8983+）。
- 点击回调按钮触发 `INTERACTION_CREATE`，**必须**调 `PUT /interactions/{interaction_id}` 回应，否则客户端一直 loading 到超时。

### 3.8 卡片与 Embed

- **ARK 结构化卡片**（[来源](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/type/ark.html)）：需向平台运营申请权限；字段 `ark.template_id`（可选 23 / 24 / 37）+ `ark.kv`（`{key:"#VAR#", value:"..."}`，数组变量用 `obj` 嵌套 `obj_kv`）。
- **图文卡片 `msg_type=8`**（群聊 / 单聊）：`card.type = "tuwen"`，`card.content` 必含 `title`(1-50 字符)、`description`(1-200 字符)、`url`(必须 https，≤1024 字符)、`pic_url`(必须 https，≤1024 字符)。
- **Embed**（仅频道）：`embed.{title, prompt, thumbnail.url, fields[].name}`。

### 3.9 明确「文档未覆盖」的消息能力

| 能力 | 结论 |
|---|---|
| **编辑 / 修改已发消息** | 官方 API v2 接口索引（[autogen](https://bot.q.qq.com/wiki/develop/api-v2/autogen/)）中**没有**编辑消息的接口，`server-inter` 目录也没有对应页。但公共错误码里存在 `3000000~3999999 编辑消息错误`、`50049 只能修改含有 keyboard 元素的消息`、`50050 修改消息时，keyboard 元素不能为空`、`50051 只能修改机器人自己发送的消息` —— 说明平台侧存在该能力，但**官方 wiki 未给出接口路径与参数**，属「文档未覆盖」。插件不要臆造 `PUT /v2/groups/.../messages/{id}` 之类的路径。 |
| **拉取历史消息列表（群聊 / 单聊）** | 文档未覆盖。只有频道的 `GET /channels/{channel_id}/messages` 在旧版 openapi 分区提及，api-v2 索引中未收录。 |
| **群聊 / 单聊的表情表态接口** | 文档未覆盖（官方明确「目前表情表态仅支持在频道内使用」）。 |
| **群聊发送时的 @ 成员语法** | 文档未覆盖（见 §3.6）。 |
| **群聊 / 单聊的 Embed 发送** | 官方明确不支持。 |
| **群聊 / 单聊发送 ARK 卡片** | 官方明确不支持（`发 ❌`，只能接收）。 |

---

## 4. 会话类型差异：单聊 / 群聊 vs 频道

### 4.1 三大场景总览

来源：[消息收发概述](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/overview.html)

| 场景 | 发送消息 | 接收事件 | 路径前缀 |
|---|---|---|---|
| QQ 单聊 | 发送单聊消息 / 流式消息 | `C2C_MESSAGE_CREATE` | `/v2/users/{user_openid}` |
| QQ 群聊 | 发送群消息 | `GROUP_AT_MESSAGE_CREATE` / `GROUP_MESSAGE_CREATE` | `/v2/groups/{group_openid}` |
| QQ 频道（文字子频道） | 发送子频道消息 / 频道私信 | `AT_MESSAGE_CREATE` 等 | `/channels/{channel_id}`、`/users/@me/dms` |

### 4.2 逐项差异

| 维度 | 群聊（QQ 群） | 频道（文字子频道） |
|---|---|---|
| 路径形态 | RESTful `/v2/groups/{group_openid}/...` | Discord 风格 `/channels/{channel_id}/...`、`/guilds/{guild_id}/...` |
| 标识 | `group_openid`（群）、`member_openid`（群成员） | `guild_id`（频道）、`channel_id`（子频道）、`user_id`（成员） |
| 发送消息消息体 | `msg_type` + `content`/`markdown`/`media`/`card` | `content`/`embed`/`ark`/`image`/`markdown`/`message_reference`（无 `msg_type`） |
| 消息类型自由度 | 0/2/6/7/8 | text / embed / ark / markdown / image / file_image（multipart） |
| **Markdown** | 收发均支持（**所有机器人可用**，无需申请模版） | 仅发送；**频道场景需内邀开通** |
| **消息按钮** | 需申请模版；自定义按钮内邀 | 需申请模版；自定义按钮内邀 |
| **表情表态** | ❌ 不支持 | ✅ 支持（PUT/DELETE/GET reactions） |
| **Embed** | ❌ | ✅ 仅发不收 |
| **文件消息** | ✅ 收发 | ❌（文档未覆盖发送文件） |
| **ARK 卡片** | ❌ 发送（可接收） | ❌ 接收（可发送，需申请权限） |
| **撤回** | `DELETE /v2/groups/{group_openid}/messages/{message_id}`，**发送超 2 分钟不可撤回**；机器人是群管理员时可撤回成员消息 | `DELETE /channels/{channel_id}/messages/{message_id}?hidetip=false`；「公域机器人暂不支持申请，仅私域机器人可用」（该提示原文出现在该页，但同页「开通后需要先将机器人从频道移除，然后重新添加」一句语义不明确，可能沿用自其它能力，**适用范围文档未覆盖**） |
| **@全体成员** | 群主权限维度管理（`allow_proactive_msg` / `recv_msg_setting`） | 频道主/管理员维度管理 |
| **权限模型** | 群主在添加机器人时设置「接收全量消息」「主动推送消息」「撤回成员消息」三项开关（[FAQ](https://bot.q.qq.com/wiki/bot_FAQ/)） | 频道主 / 频道管理员，子频道权限、身份组权限体系完整 |
| **主动消息审核** | 无此机制 | 有：`MESSAGE_AUDIT` intent 事件返回 `MessageAudited`；错误码 `304023/304024` 携带 `audit_id` |
| **在线要求** | 无 WS 在线硬要求 | **「发送消息接口要求机器人接口需要连接到 websocket 上保持在线状态」** |
| **分片** | 无（按群路由） | 按 `guild_id` 哈希分片 |

### 4.3 群聊独有能力（对论坛插件尤其有用）

| 能力 | 接口 | 说明 |
|---|---|---|
| 群基本信息 | `GET /v2/groups/{group_openid}/info` | 群名、简介、标签、成员数（**白名单机器人可用**） |
| 机器人群内状态 | `GET /v2/groups/{group_openid}/bot_state` | `allow_proactive_msg`（是否接收主动推送）、`recv_msg_setting`（`all` / `only_mention` / `mention_and_context`）、`member_role`（**白名单机器人可用**） |
| 群成员列表 / 信息 | `GET /v2/groups/{group_openid}/members` | 每页最多 30 条，游标分页（**内邀中，白名单**） |
| 批量移除成员 | `POST /v2/groups/{group_openid}/batch_remove_members` | 内邀中 |
| 群黑名单 | `GET/POST /v2/groups/{group_openid}/member_blacklist` | 内邀中 |
| 群禁言 | `GET/POST /v2/groups/{group_openid}/restrict_chat_setting` | 内邀中 |
| 入群申请审批 / 自动审批策略 | `GET /v2/groups/{group_openid}/join_request_list`、`POST .../approval_join_request/{member_openid}`、`POST/PATCH/DELETE /v2/groups/join_approval_strategy...` | 含白名单号码管理 |

### 4.4 频道独有能力

- 子频道 CRUD：`GET/POST /guilds/{guild_id}/channels`、`GET/PATCH/DELETE /channels/{channel_id}`
- 公告：`POST/DELETE /guilds/{guild_id}/announces`、`POST/DELETE /channels/{channel_id}/announces`
- 精华：`POST/GET/DELETE /channels/{channel_id}/pins/{message_id}`
- 论坛（频道帖）：`PUT/GET/DELETE /channels/{channel_id}/threads`
- 日程：`POST/GET/PATCH/DELETE /channels/{channel_id}/schedules`
- 角色 / 成员：`GET/PUT/DELETE /guilds/{guild_id}/roles...`、`/members...`
- 权限：`GET/PUT /channels/{channel_id}/members/{user_id}/permissions`、`/roles/{role_id}/permissions`
- 音频：`PUT/DELETE /channels/{channel_id}/mic`、静音 `PATCH /guilds/{guild_id}/members/{user_id}/mute`
- 频道私信：`POST /users/@me/dms` + 在该会话中发消息
- 子频道 API 权限申请：`POST /guilds/{guild_id}/api_permissions`（自助申请接口权限）

> **注意**：这些频道侧接口全部只在 `server-inter/channel/**` 旧版 openapi 分区有文档页，**不在 api-v2 的 `autogen` 接口索引中**。做能力判断时不能只看 autogen 索引。

---

## 5. 限频与配额

### 5.1 接口级频控（各接口页「接口频率限制」字段）

| 接口 | 频控 |
|---|---|
| `POST /v2/groups/{group_openid}/messages` | 100 QPS |
| `POST /v2/users/{user_openid}/messages` | 100 QPS |
| `DELETE /v2/groups/.../messages/{message_id}` | 10 QPS |
| `DELETE /v2/users/.../messages/{message_id}` | 10 QPS |
| `POST /v2/users/{user_openid}/stream_messages` | 50 QPS |
| `POST /v2/groups/{group_openid}/files` / `POST /v2/users/{user_openid}/files` | 50 QPS |
| `POST /v2/{groups,users}/.../upload_prepare` | 10 QPS |
| `POST /v2/{groups,users}/.../upload_part_finish` | 10 QPS |
| `GET /gateway` | 2 QPM / 10 QPM burst |
| `GET /users/@me` | 50 QPS |
| `PUT /interactions/{interaction_id}` | 50 QPS |
| `POST /v2/generate_url_link` | 50 QPS |
| `GET /v2/groups/{group_openid}/info` / `/bot_state` | 30 QPM |
| `GET /v2/groups/{group_openid}/members` | 60 QPM |
| `GET /v2/menu` / `PUT /v2/menu` | 30 QPM / 5 QPM |
| `GET /v2/panels` / `POST /v2/panels` | 30 QPM / 10 QPM |
| `POST /app/getAppAccessToken` | **文档未覆盖** |
| 全部频道侧接口（`/channels/**`） | **文档未覆盖** |

### 5.2 主动消息 / 被动消息的时间窗差异

来源：[消息收发概述 · 主动消息与被动消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/overview.html)

| 类型 | 特征 | 说明 |
|---|---|---|
| 主动消息 | 无任何条件 | 机器人主动触达用户；**用户可在客户端关闭「允许主动发送」开关，关闭后主动消息一律发送失败** |
| 互动召回消息 | `is_wakeup=true` | 用户主动与机器人对话之后可下发；共 4 个周期（当天 / 1-3 天 / 3-7 天 / 7-30 天），**每周期 1 条**；仅单聊 |
| 被动消息（回复用户） | 携带 `msg_id` | 对用户消息的回复 |
| 被动消息（响应事件） | 携带 `event_id` | 对事件的回复 |

**被动消息有效期与回复次数**

| 场景 | 有效期 | 每条消息可回复次数 |
|---|---|---|
| 单聊 | 60 分钟 | 4 次 |
| 群聊 | 5 分钟 | 5 次 |
| 频道 | 5 分钟 | — |

**主动消息频控（HTTP 接口）**

单聊：

| 认证类型 | Bot 维度频控 | 单关系维度频控 | 每日上限 |
|---|---|---|---|
| 企业认证 | 10/qps | 20/qpm | 1000 条/用户 |
| 个人认证 | 10/qps | 20/qpm | 1000 条/用户 |
| 未认证 | 5/qps & 30/qpm | 20/qpm | 1000 条/用户 |

群聊：

| 认证类型 | Bot 维度频控 | 单关系维度频控 | 每日上限 |
|---|---|---|---|
| 企业认证 | 60/qpm | 20/qpm | 1000 条/群 |
| 个人认证 | 60/qpm | 20/qpm | 1000 条/群 |
| 未认证 | 30/qpm | 20/qpm | 1000 条/群 |

文字子频道：

- 主动推送消息，默认每天往**每个子频道**可推送 **20 条**。
- 主动推送消息在**每个频道**中，每天可以往 **2 个子频道**推送消息。
- 「不论主动消息还是被动消息，在一个子频道中，**每秒最多可发送 5 条**消息。」
- 被动回复消息有效期 **5 分钟**，超时会发送失败。
- 发送消息接口要求机器人**连接到 WebSocket 保持在线**。

频道私信：

- 每个机器人每天可以对**一个用户发 2 条**主动消息。
- 每个机器人每天**累计 200 条**主动消息。

### 5.3 其它配额与限制

| 项 | 数值 | 来源 |
|---|---|---|
| 消息去重 | 「相同 msg_id 可能多次推送，请结合 msg_seq 去重。被动回复时，相同的 msg_id + msg_seq 重复发送会失败」 | [消息收发概述](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/overview.html) |
| 撤回时限 | 发送超过 **2 分钟**的消息不可撤回 | [撤回群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages_message_id.delete.html) |
| 消息长度 | 错误码 `40054007 消息长度超限` / `40054018 消息过长或异常`；**具体字数上限文档未覆盖** | [API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html) |
| 富媒体容量 | 错误码 `40093002 超过今天发送文件容量上限`；**具体数值文档未覆盖** | [群聊富媒体上传](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_files.post.html) |
| Markdown 素材 | 单机器人 100 张；滑动过期 90 天；上传任务 4 小时有效 | [素材管理](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/material-management.html) |
| 每日日程 | 「每天只能创建 10 个日程」；单日程不超过 7 天 | [API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html) |
| Gateway Session | `session_start_limit.total` = 每 24 小时可创建 Session 数，`max_concurrency` = 每 5s 可创建 Session 数 | [获取带分片 WSS 接入点](https://bot.q.qq.com/wiki/develop/api-v2/openapi/wss/shard_url_get.html) |

### 5.4 频控相关的关键错误码

| 错误码 | 含义 |
|---|---|
| `304022 PUSH_TIME` | 推送消息时间限制 |
| `304045` / `304046` | 子频道主动消息数限频 / 不允许在此子频道发主动消息 |
| `304047` / `304048` | 主动消息推送超过限制的子频道数 / 不允许在此频道发主动消息 |
| `304049` / `304050` | 私信主动消息数限频 / 私信主动消息总量限频 |
| `20028 ChannelHitWriteRateLimit` | 子频道消息触发限频 |
| `40034100` | 主动消息发送超过频控限制 |
| `40034122` / `40034123` | 召回消息已达区间上限 / 不支持召回消息 |
| `40034128` | 被动回复时间或次数超限 |
| `1100100` | 安全打击：消息被限频 |
| `1100308` | 触发频道内限频 |
| `304023` / `304024` | 推送/回复消息异步调用成功，等待人工审核（body 携带 `audit_id`） |

---

## 6. 沙箱环境与正式环境

> **重要发现**：官方 wiki **没有一页专门讲沙箱 vs 正式环境**。以下是把分散在 FAQ、WS 错误码、频道事件页与运营规范里的相关原文汇总；凡是没有官方出处的，一律标注「文档未覆盖」。

### 6.1 官方明确写到的

| 主题 | 官方原文 / 结论 | 来源 |
|---|---|---|
| **是否还需要沙箱环境** | 「使用管理员账号进入新版机器人管理端后，管理员账号可直接测试使用，**无需沙箱环境**。（旧版管理端已不支持添加沙箱群\频道配置）」 | [FAQ · 怎么添加沙箱环境测试机器人](https://bot.q.qq.com/wiki/bot_FAQ/) |
| **机器人数量限制** | 「个人主体：个人认证开发者，支持创建 **5 个**机器人。企业主体：企业认证开发者，支持创建 **10 个**机器人；同一企业主体下所有认证账号共用机器人创建名额，创建上限为 **50 个**。」 | [FAQ · 为什么只能创建 5 个机器人](https://bot.q.qq.com/wiki/bot_FAQ/) |
| **公开可用范围** | 「目前在最新机器人管理端中，**企业主体公开使用已开放，个人主体暂不支持公开使用**。个人主体的机器人如需添加群聊，机器人创建者可直接添加自己是群主/管理员身份的群聊（升级最新版移动QQ添加）。」 | [FAQ · 机器人如何公开添加进群、频道](https://bot.q.qq.com/wiki/bot_FAQ/) |
| **私聊测试用户** | 「个人主体的机器人如需添加人员私聊使用，可以在**新版管理端「开发设置」中添加开发体验用户号码**。」 | 同上 |
| **私信事件不支持沙箱** | 「由于私信场景无法设置沙箱频道，目前私信事件不支持沙箱环境，开发者可以通过**用户 id 白名单**的方式来调试私信」 | [频道消息事件](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/event.html) |
| **下架机器人的网关行为** | WS 错误码 `4914 机器人已下架，只允许连接沙箱环境，请断开连接，检验当前连接环境` | [WebSocket 错误码](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/error-trace/websocket.html) |
| **旧版管理端已停用** | 「旧版管理端目前已停止审核上架、功能配置等，请使用机器人管理员登录新版QQ Bot管理端」 | [FAQ · 旧版管理端机器人提交上架审核](https://bot.q.qq.com/wiki/bot_FAQ/) |
| **公开服务开关** | 「新注册账号审核预计 **3 天**完成，请耐心等待后尝试」；未完成钱包实名认证也会导致无法开启公开服务 | [FAQ · 打开公开服务按钮失败](https://bot.q.qq.com/wiki/bot_FAQ/) |
| **群侧能力白名单** | 群信息 / bot_state / 群成员列表 / 黑名单 / 批量移除等接口错误码统一为 `11253 应用无接口访问权限 —— 该接口仅白名单机器人可用，请联系平台运营申请权限` | [获取群基本信息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_info.get.html) 等 |
| **频道 Markdown / 消息按钮** | 单聊、群聊场景的自定义 Markdown 与消息按钮「已开放到所有机器人均可使用，无需单独申请 Markdown 模版」；「频道场景目前需要内邀开通」；自定义按钮「内邀开通」 | [Markdown 消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/type/markdown.html)、[消息按钮](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/trans/msg-btn.html) |
| **公域 / 私域机器人** | 多个能力按「公域机器人」与「私域机器人」区分（如 `MESSAGE_CREATE` 仅私域可订阅、频道撤回消息「公域机器人暂不支持申请，仅私域机器人可用」）。**两种机器人的申请流程与准入条件：文档未覆盖** | [通用数据结构](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/payload.html)、[撤回子频道消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/recall.html) |
| **发布审核** | [运营规范](https://bot.q.qq.com/wiki/business/) 第 1 节「注册提交规范」：需提供有效联系方式、实名管理员 QQ 号、平台要求的补充材料；「不允许批量注册、重复提交大量相似的 QQ 机器人」。第 4 节「主体规范」：需通过发布审核 | [运营规范](https://bot.q.qq.com/wiki/business/) |

### 6.2 明确「文档未覆盖」的沙箱/上线条目

| 条目 | 结论 |
|---|---|
| 沙箱环境与正式环境的 **API 域名差异** | **文档未覆盖**。官方只给出统一基址 `https://api.bot.qq.com` 与 `wss://api.bot.qq.com/websocket/`，没有第二套沙箱域名。`4914` 错误码只说「检验当前连接环境」，没说环境如何区分。 |
| 沙箱环境的**群/频道白名单配置项位置与字段** | **文档未覆盖**。FAQ 明确说「旧版管理端已不支持添加沙箱群\频道配置」。 |
| **机器人上架审核的具体流程步骤、提交入口、审核时长** | **文档未覆盖**（FAQ 只说旧版管理端已停止审核上架、功能配置；新注册账号审核预计 3 天完成）。 |
| **沙箱机器人的群数量上限** | **文档未覆盖**。 |
| **沙箱与正式的频控/配额是否不同** | **文档未覆盖**。 |
| **测试环境专用的 mock/回放入口** | **文档未覆盖**。 |
| **「私域机器人」的申请条件与流程** | **文档未覆盖**（仅散见「内邀」「白名单」字样）。 |
| **白名单接口的申请入口** | 仅错误码写「请联系平台运营申请权限」，**申请入口/表单文档未覆盖**。 |

### 6.3 上线前的必要步骤（据官方事实归纳）

1. **注册并认证主体**：个人认证或企业认证；个人主体 5 个机器人、企业主体 10 个（同一主体合计上限 50）。[FAQ](https://bot.q.qq.com/wiki/bot_FAQ/)
2. **创建机器人并取得 AppID / AppSecret**：[启动接入](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/getting-started.html)
3. **调试阶段**：用管理员账号在**新版 QQ Bot 管理端**直接测试，无需沙箱；个人主体可用「开发设置 → 开发体验用户号码」添加私聊测试用户。[FAQ](https://bot.q.qq.com/wiki/bot_FAQ/)
4. **把机器人加进目标群**：个人主体机器人需创建者本人是群主/管理员；并在群成员列表里为机器人配置三项开关——**接收全量消息**、**主动推送消息**、**撤回成员消息**（第三项需机器人被设为群管理员）。[FAQ](https://bot.q.qq.com/wiki/bot_FAQ/)
5. **确认群侧消息开关**：用 `GET /v2/groups/{group_openid}/bot_state` 检查 `allow_proactive_msg` 与 `recv_msg_setting`（需白名单机器人）。[获取机器人群内状态](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_bot_state.get.html)
6. **若要上频道**：Markdown 与自定义消息按钮需**内邀开通**；频道发消息要求 **WS 保持在线**。[Markdown 消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/type/markdown.html)
7. **开通公开服务（正式对外）**：企业主体已开放，个人主体暂不支持；新注册账号审核约 3 天；需完成钱包实名认证。[FAQ](https://bot.q.qq.com/wiki/bot_FAQ/)
8. **合规自查**：[运营规范](https://bot.q.qq.com/wiki/business/) 的注册提交规范、基本信息规范（名称/头像禁含联系方式与极限用语）、功能设置规范、内容安全与数据合规要求。

---

## 7. 论坛 ↔ QQ 能力映射

### 7.1 论坛事件 → QQ 推送（出向）

| 论坛侧动作 | 推荐 QQ 能力 | 接口 | 注意事项 |
|---|---|---|---|
| 新帖 / 新回复通知 | 群聊文本 / Markdown | `POST /v2/groups/{group_openid}/messages`（`msg_type=0` 或 `2`） | **只能主动推送**：受 60/qpm（企业/个人）或 30/qpm（未认证）Bot 维度频控 + 20/qpm 单关系频控 + 1000 条/群/天 限制；且群主必须开启「允许主动推送消息」（否则 `allow_proactive_msg=false`，主动消息失败） |
| 公告 / 审核结果 | 群聊图文卡片 | 同上，`msg_type=8` + `card.type=tuwen` | `url` / `pic_url` 必须 https |
| 富文本帖（带封面图） | 群聊富媒体 | `POST /v2/groups/{group_openid}/files` → `media.file_info` → `msg_type=7` | 图片软限 20 MB；`file_info` 有 ttl，需缓存复用 |
| Markdown 正文 + 内联按钮 | 群聊 `markdown` + `keyboard` | 同上，`msg_type=2` + `keyboard.content.rows` | 自定义按钮需内邀；按钮回调触发 `INTERACTION_CREATE`，必须回 `PUT /interactions/{interaction_id}` |
| 频道公告 | 频道公告接口 | `POST /guilds/{guild_id}/announces` / `POST /channels/{channel_id}/announces` | 需 WS 在线 |
| 频道富文本 | 频道 `markdown` 或 `embed` | `POST /channels/{channel_id}/messages` | 频道 Markdown 需内邀 |
| 订阅消息（用户级提醒） | 互动召回消息 | `POST /v2/users/{user_openid}/messages` + `is_wakeup=true` | 4 个周期 × 每周期 1 条；与 `msg_id`/`event_id` 互斥 |
| 生成邀请链接 | 分享链接 | `POST /v2/generate_url_link`（`callback_data` ≤32 字符） | 用户通过链接加好友时 `callback_data` 透传，可用于绑定论坛账号 |

> **单聊不适合做「论坛广播」**：单聊主动消息上限是 1000 条/用户/天且频控更严（10/qps 或 5/qps&30/qpm），适合一对一答疑，不适合群发。
> **论坛广播首选群聊**（频控 60/qpm Bot 维度、1000 条/群/天），但「天级上限」意味着论坛日发帖量超过 1000 时必须做合并/摘要推送。

### 7.2 QQ 消息 → 论坛回流（入向）

| QQ 侧输入 | 事件 | 论坛侧动作 | 注意事项 |
|---|---|---|---|
| 群内 @机器人 | `GROUP_AT_MESSAGE_CREATE`（intent `1<<25`） | 发帖 / 回帖 / 查帖 | `content` 已去掉 @机器人前缀；`author.member_openid` 是群内唯一标识；需按 `msg_id`+`msg_seq` 去重；被动回复窗口 5 分钟 / 5 次 |
| 群内普通消息（全量模式） | `GROUP_MESSAGE_CREATE` | 采集/审计/统计 | 需群主开启「接收所有消息」（`recv_msg_setting=all`）；否则机器人只收到 @ 消息 |
| 单聊消息 | `C2C_MESSAGE_CREATE` | 一对一答疑 / 建档 | 被动回复窗口 **60 分钟 / 4 次**；单聊需先成为好友 |
| 消息按钮点击 | `INTERACTION_CREATE`（`type=11`） | 执行命令（发帖、翻页、审批） | 必须回 `PUT /interactions/{interaction_id}`（`code=0`），否则客户端 loading 到超时；`interaction_id` 只能回应一次 |
| 快捷菜单点击 | `INTERACTION_CREATE`（`type=12`） | 执行命令 | 同上需回应 |
| 消息反馈（赞/踩） | `INTERACTION_CREATE`（`type=13`） | 回流为论坛反馈 | 官方说明无需回应 |
| 群授权 | `INTERACTION_CREATE`（`type=19/20`） | 记录用户对主动推送的授权状态 | 可与 `bot_state.allow_proactive_msg` 交叉验证 |
| 表情表态（频道） | `MESSAGE_REACTION_ADD/REMOVE` | 回流为点赞/点踩 | 仅频道支持 |
| 群成员进出 | `GROUP_MEMBER_ADD/REMOVE` | 同步论坛成员 | — |
| 机器人被加/移出群 | `GROUP_ADD_ROBOT/GROUP_DEL_ROBOT` | 维护推送目标表 | 这两个事件可直接作为被动回复的 `event_id` |
| 频道消息（@ 或全量） | `AT_MESSAGE_CREATE` / `MESSAGE_CREATE`（私域） | 频道内指令 | `MESSAGE_CREATE` 仅私域机器人可订阅 |

### 7.3 实现决策建议（依据上文事实）

1. **通道**：WebSocket 为主（出站、免公网入口、频道发消息的硬前提），Webhook 为可选高级选项（需用户自备公网 HTTPS 回调地址）。
2. **主场景**：群聊（`GROUP_AND_C2C_EVENT` intent 一把抓）+ 单聊。频道能力按需再开（Markdown/自定义按钮需内邀）。
3. **推送侧**：主动推送受群主开关与天级 1000 条/群限制，论坛侧需要「合并 + 摘要」策略，不能一帖一条无限推。
4. **回流侧**：以 `member_openid` 作为 QQ 侧用户主键；跨 AppID 的论坛账号绑定只能靠 `union_openid`（需邮件申请）或插件自建绑定表（走宿主 Command 契约，不碰宿主数据库）。
5. **幂等**：所有入向事件按 `(msg_id, msg_seq)` 或 `message_scene.ext` 的 `msg_idx` 去重；所有出向被动回复必须递增 `msg_seq`。
6. **身份打通**：群聊里同一个人在不同群的 `member_openid` 不同，不能直接当全局用户 ID —— 需要插件侧维护 `member_openid → 论坛用户` 的映射。

---

## 8. 汇总：查不到或文档未覆盖的条目

以下条目在官方 API v2 文档中**没有可用答案**，实现时必须按「未覆盖」处理，不得臆造：

> 另有一类「wiki 没有、但官方 SDK 有」的接口：扫码绑定（`q.qq.com/lite/*`，见 本文件 §11）与 npm SDK 里的 QQ 机器人入口。它们同样没有 wiki 文档页，使用前按「平台可能变更」对待。

| # | 条目 | 说明 |
|---|---|---|
| 1 | **编辑 / 修改已发消息**的接口路径与参数 | 索引中无该接口；仅错误码 `3000000~3999999 编辑消息错误`、`50049/50050/50051` 暗示能力存在 |
| 2 | 群聊 / 单聊的**历史消息列表**接口 | 文档未覆盖 |
| 3 | 群聊 / 单聊的**表情表态**接口 | 官方明确仅频道支持 |
| 4 | 群聊**发送消息时 @ 某成员**的语法 | 文档未覆盖（只有频道的 `<@!user_id>` 语法） |
| 5 | **消息内容长度上限**（字数） | 只有错误码 `40054007` / `40054018`，无具体数值 |
| 6 | **每日文件发送容量上限** | 只有错误码 `40093002`，无具体数值 |
| 7 | **`getAppAccessToken` 的频控数值** | 只有错误码 `100001 Too many requests` |
| 8 | **全部频道侧接口（`/channels/**`、`/guilds/**`）的 QPS/QPM** | 文档未覆盖 |
| 9 | **沙箱环境与正式环境的 API 域名差异** | 文档未覆盖（只有统一域名） |
| 10 | **沙箱群/频道白名单的配置位置与字段** | 文档未覆盖（旧版管理端已下线该能力） |
| 11 | **上架审核的具体流程与入口** | 文档未覆盖（仅运营规范的原则性描述 + FAQ「审核预计 3 天」） |
| 12 | **沙箱环境的机器人数量/群数量上限** | 文档未覆盖 |
| 13 | **沙箱与正式环境的频控是否不同** | 文档未覆盖 |
| 14 | **「私域机器人」申请条件与流程** | 文档未覆盖（只有「内邀」「白名单」字样） |
| 15 | **白名单接口的申请入口** | 仅「联系平台运营」，无表单文档 |
| 16 | **官方对 WebSocket vs Webhook 的明确推荐** | 文档未覆盖（两页并列，无推荐语） |
| 17 | **官方 SDK 的具体版本与 API 契约** | 索引页只给出 `botgo` / `botpy` / `bot-node-sdk` 三个链接与「SDK 仅提供接入参考，详情能力以官方文档能力描述为准」，未给出版本号或兼容性承诺 |
| 18 | **图文卡片的 `msg_type` 到底是 3 还是 8** | 官方两处表述冲突：[消息类型](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/type/overview.html) 末尾写「发送侧只有 msg_type=0/2/3/7 四种」（但同页表格只列 0/2/7），而 [发送群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html) 的请求体说明写 `8=图文卡片(card)`。实现时以接口页为准（`msg_type=8` + `card` 字段），并对 3 做兼容兜底 |
| 19 | **群聊主动消息的「天级 1000 条/群」是否与认证类型相关** | 表格中三种认证类型均为 1000 条/群，但未说明未认证机器人是否有更严限制 |
| 20 | **@全体成员（`304019`）的次数上限** | 只有错误码，无具体数值 |

---

## 9. 可直接运行的调用示例

> 以下示例中的 `APPID` / `CLIENTSECRET` / `ACCESS_TOKEN` / `GROUP_OPENID` / `USER_OPENID` 为占位符，替换后即可执行。curl 示例直接摘自官方文档页。

### 示例 1：获取 access_token

来源：[获取访问凭证](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html)

```bash
curl --location 'https://api.bot.qq.com/app/getAppAccessToken' \
--header 'Content-Type: application/json' \
--data '{
 "appId": "APPID",
 "clientSecret": "CLIENTSECRET"
}'
```

成功返回：

```json
{
 "access_token": "ACCESS_TOKEN",
 "expires_in": "7200"
}
```

失败（**HTTP 仍是 200**）：

```json
{
 "code": 100007,
 "message": "appid invalid"
}
```

### 示例 2：取 WSS 接入点并取机器人信息

来源：[获取通用 WSS 接入点](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/gateway.get.html)、[API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html)

```bash
# 取 WebSocket 网关地址（频控 2 QPM / 10 QPM burst，不要高频轮询）
curl -s 'https://api.bot.qq.com/gateway' \
  -H 'Authorization: QQBot ACCESS_TOKEN'

# 期望输出：{"url": "wss://api.bot.qq.com/websocket/"}

# 取机器人自身信息
curl -s 'https://api.bot.qq.com/users/@me' \
  -H 'Authorization: QQBot ACCESS_TOKEN'
```

```json
{
 "id": "5777414462219517083",
 "username": "阳光小助手",
 "avatar": "https://thirdqq.qlogo.cn/g?b=oidb&k=...",
 "bot": true,
 "share_url": "https://qun.qq.com/qunpro/robot/qunshare?robot_uin=3889007780&robot_appid=102083127&biz_type=0",
 "welcome_msg": "欢迎加入我们的群聊"
}
```

### 示例 3：发送群聊消息（被动回复，带 `msg_id` + `msg_seq`）

来源：[发送群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html)

```bash
curl -X POST 'https://api.bot.qq.com/v2/groups/GROUP_OPENID/messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "msg_type": 0,
    "content": "欢迎使用本群助手，有什么可以帮你的吗？",
    "msg_id": "ROBOT1.0_xxxxxx",
    "msg_seq": 1
  }'
```

成功：

```json
{
 "id": "ROBOT1.0_a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
 "timestamp": "2026-07-21T10:00:00+08:00",
 "ext_info": { "ref_idx": "REFIDX_xxxxxxxxxxxxxxx==" }
}
```

失败：

```json
{
 "err_code": 40034005,
 "message": "回复消息msg_id已过期",
 "trace_id": "4a8a61565b909f199b1ec169fdd6f49e"
}
```

### 示例 4：发送群聊 Markdown + 内联键盘消息

来源：[发送群聊消息 · Markdown + 键盘消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html)

```bash
curl -X POST 'https://api.bot.qq.com/v2/groups/GROUP_OPENID/messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "msg_type": 2,
    "markdown": {
      "content": "## 每日签到\n\n今日签到成功！获得 **50** 积分\n连续签到 **7** 天"
    },
    "keyboard": {
      "content": {
        "rows": [
          {
            "buttons": [
              {
                "id": "btn_signin",
                "render_data": { "label": "签到", "style": 1 },
                "action": {
                  "type": 2,
                  "permission": { "type": 2 },
                  "data": "/签到",
                  "enter": true
                }
              }
            ]
          }
        ]
      }
    },
    "msg_id": "ROBOT1.0_xxxxxx",
    "msg_seq": 1
  }'
```

### 示例 5：单聊消息 + 流式发送

来源：[发送单聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_messages.post.html)、[流式发送单聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_users_user_openid_stream_messages.post.html)

```bash
# 普通单聊
curl -X POST 'https://api.bot.qq.com/v2/users/USER_OPENID/messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "content": "你好，欢迎使用机器人助手！",
    "msg_type": 0,
    "msg_id": "ROBOT1.0_xxxxxx",
    "msg_seq": 1
  }'

# 流式首片（服务端返回 stream_msg_id）
curl -X POST 'https://api.bot.qq.com/v2/users/USER_OPENID/stream_messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "input_mode": "replace",
    "input_state": 1,
    "index": 0,
    "content_type": "markdown",
    "content_raw": "正在生成回答，请稍候",
    "msg_id": "ROBOT1.0_xxxxxx",
    "msg_seq": 1
  }'

# 流式续片（携带上一片返回的 stream_msg_id，index 递增）
curl -X POST 'https://api.bot.qq.com/v2/users/USER_OPENID/stream_messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "input_mode": "replace",
    "input_state": 1,
    "index": 1,
    "content_type": "markdown",
    "content_raw": "正在生成回答，请稍候。目前已完成大部分内容",
    "msg_id": "ROBOT1.0_xxxxxx",
    "stream_msg_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "msg_seq": 1
  }'
```

### 示例 6：群聊图片上传（URL 方式）→ 发送富媒体消息

来源：[群聊富媒体上传](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_files.post.html)、[富媒体消息概述](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/rich-media.html)

```bash
# 第一步：上传（返回 file_info，注意 ttl）
curl -X POST 'https://api.bot.qq.com/v2/groups/GROUP_OPENID/files' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "file_type": 1,
    "url": "https://example.com/image.png",
    "srv_send_msg": false
  }'

# {"file_uuid": "uuid_a1b2c3d4e5f6", "file_info": "AE86C5D3F0E14B238C656C0F6DD1D0479C", "ttl": 300}

# 第二步：用 file_info 发富媒体消息
curl -X POST 'https://api.bot.qq.com/v2/groups/GROUP_OPENID/messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "msg_type": 7,
    "media": { "file_info": "AE86C5D3F0E14B238C656C0F6DD1D0479C" },
    "msg_id": "ROBOT1.0_xxxxxx",
    "msg_seq": 2
  }'
```

### 示例 7：发送子频道消息（频道）

来源：[发送子频道消息](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/channel/message/send.html)

```bash
curl -X POST 'https://api.bot.qq.com/channels/CHANNEL_ID/messages' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{
    "content": "<@!1234>hello world",
    "msg_id": "xxxxxx"
  }'
```

### 示例 8：互动事件回应（收到 `INTERACTION_CREATE` 后必须调用）

来源：[互动事件响应](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/interactions_interaction_id.put.html)

```bash
curl -X PUT 'https://api.bot.qq.com/interactions/a1b2c3d4-e5f6-7890-abcd-ef1234567890' \
  -H 'Authorization: QQBot ACCESS_TOKEN' \
  -H 'Content-Type: application/json; charset=utf-8' \
  -d '{ "code": 0 }'
```

### 示例 9：入向事件结构（插件需要解析的 payload）

来源：[群@机器人消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_at_message_create.html)、[单聊消息事件](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/c2c_message_create.html)、[通用数据结构](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/payload.html)

WebSocket 下行消息外层结构：

```json
{
  "id": "event_id",
  "op": 0,
  "d": {},
  "s": 42,
  "t": "GROUP_AT_MESSAGE_CREATE"
}
```

`d`（群 @ 机器人消息）：

```json
{
  "id": "ROBOT1.0_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "author": {
    "id": "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4",
    "member_openid": "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4",
    "member_role": "member",
    "username": "小明",
    "bot": false
  },
  "content": "/今日天气",
  "group_openid": "B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5",
  "message_type": 0,
  "timestamp": "2026-07-21T10:00:00+08:00",
  "message_scene": {
    "source": "default",
    "ext": [
      "msg_idx=REFIDX_xxxxxxxxxxxxxxx==",
      "auth_token=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    ]
  }
}
```

`d`（单聊消息，含 ARK 卡片）：

```json
{
  "id": "ROBOT1.0_yyyy...",
  "author": {
    "id": "B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5",
    "user_openid": "B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5",
    "union_openid": "",
    "username": "",
    "bot": false
  },
  "content": "[卡片消息] 小程序\n摘要: [每日打卡]快来完成今日学习打卡",
  "message_type": 3,
  "ark_data": {
    "ark_type": "miniapp",
    "ark_name": "小程序",
    "prompt": "[每日打卡]快来完成今日学习打卡",
    "fields": { "title": "快来完成今日学习打卡", "source": "学习助手" }
  },
  "message_scene": { "source": "default", "ext": ["msg_idx=REFIDX_xxxxxxxxxxxxxxx=="] },
  "timestamp": "2026-07-21T10:01:00+08:00"
}
```

### 示例 10：Webhook 回调地址验证（op=13）应答

来源：[Webhook 方式](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/event-emit/webhook.html)

平台请求：

```json
{ "d": { "plain_token": "Arq0D5A61EgUu4OxUvOp", "event_ts": "1725442341" }, "op": 13 }
```

机器人应答：

```json
{
  "plain_token": "Arq0D5A61EgUu4OxUvOp",
  "signature": "87befc99c42c651b3aac0278e71ada338433ae26fcb24307bdc5ad38c1adc2d01bcfcadc0842edac85e85205028a1132afe09280305f13aa6909ffc2d652c706"
}
```

签名计算（Go，等价于官方示例）：

```go
seed := botSecret
for len(seed) < ed25519.SeedSize {
    seed = strings.Repeat(seed, 2)
}
seed = seed[:ed25519.SeedSize]
reader := strings.NewReader(seed)
_, privateKey, _ := ed25519.GenerateKey(reader)

var msg bytes.Buffer
msg.WriteString(eventTs)
msg.WriteString(plainToken)
signature := hex.EncodeToString(ed25519.Sign(privateKey, msg.Bytes()))
```

事件回调验签（Go，等价于官方示例）：

```go
signature := req.Header.Get("X-Signature-Ed25519")
sig, err := hex.DecodeString(signature)
if err != nil || len(sig) != ed25519.SignatureSize || sig[63]&224 != 0 {
    return false
}
timestamp := req.Header.Get("X-Signature-Timestamp")
httpBody, _ := io.ReadAll(req.Body)

var msg bytes.Buffer
msg.WriteString(timestamp)
msg.Write(httpBody)

ok := ed25519.Verify(publicKey, msg.Bytes(), sig)
```

---

## 10. 附：调研覆盖范围

- 抓取 `https://bot.q.qq.com/wiki/sitemap.xml`，共 **423** 个文档 URL。
- 其中 `develop/api-v2/**` 共 **192** 页（全部抓取并转文本），其余 **205** 页（含 FAQ、运营规范、三个语言 SDK 文档）同样抓取，用于交叉核对。
- 逐页核对的重点章节：鉴权、事件订阅（WS/Webhook/payload）、消息收发概述、消息类型、消息子类型（markdown/ark/embed）、富媒体与素材、消息转换（emoji/msg-btn/text-chain）、频道消息（send/recall/format/event/dms）、`autogen` 接口索引与全部 `autogen/api/*`、`autogen/event/*` 页、`openapi/*` 页。
- 快照时间以各页脚注「上次更新」为准，最新为 `2026-11-11`。官方文档会持续更新（本次抓取期间已见到 `2026-10-21`、`2026-10-28`、`2026-11-11` 等多次修订），**实现前建议重新核对本文标注的链接**。

---

## 11. 扫码绑定（q.qq.com lite）

> **来源与风险（先读这段）**：本节**不是**官方 wiki 内容。端点、字段与二维码 URL 形态来自腾讯
> 官方 npm 包 **`@tencent-connect/qqbot-connector@1.2.0`**（2026-11-18 读取其发布产物核对，
> 见 <https://www.npmjs.com/package/@tencent-connect/qqbot-connector>）。官方 wiki
> （`bot.q.qq.com/wiki`，本轮抓取 423 页）**没有这两个端点的文档页**，也没有版本兼容承诺，
> 频控与错误码语义均未公开。平台可能随时调整字段或下线。
> 因此 `sforum.qq-bot` 把基址、连接页与超时都做成可注入字段，失败一律降级为
> 「可先手动填凭据」，且不把这两个端点当作推送链路的依赖。

### 11.1 两个端点

| 用途 | 方法与路径 | 基址 |
|---|---|---|
| 申请绑定任务 | `POST /lite/create_bind_task` | `https://q.qq.com` |
| 轮询绑定结果 | `POST /lite/poll_bind_result` | `https://q.qq.com` |

请求与响应都是 JSON，外层信封 `{ "retcode": 0, "msg": "...", "data": {...} }`；
`retcode != 0` 即业务失败（本插件不重试，避免更快撞限流）。

**申请绑定任务**

```json
// 请求
{ "key": "<base64(32 字节 CSPRNG)>" }
// 响应 data
{ "task_id": "<string>" }
```

`key` 由调用方生成（32 字节 CSPRNG 的 base64），只在内存里保存。平台把它与这次二维码绑定，
扫码完成后用它加密下发的凭据。`task_id` 是后续轮询的唯一凭据。

**轮询绑定结果**

```json
// 请求
{ "task_id": "<string>" }
// 响应 data
{ "status": 0, "bot_appid": "<string>", "bot_encrypt_secret": "<base64 密文>" }
```

| `status` | 含义 | 有效字段 |
|---|---|---|
| `0` | 未扫码 | — |
| `1` | 已扫码，待手机侧确认 | — |
| `2` | 已完成 | `bot_appid`、`bot_encrypt_secret` |
| `3` | 已过期 | — |

### 11.2 二维码内容

```text
https://q.qq.com/qqbot/openclaw/connect.html?task_id=<task_id>&source=<调用方标记>&_wv=2
```

手机 QQ 扫码后打开此页完成授权；`source` 是调用方标记（本插件传 `sforum`），`_wv=2` 按协议固定携带。
参数顺序固定为 `task_id` → `source` → `_wv`。

### 11.3 凭据解密（AES-256-GCM）

```text
key      = base64decode(create_bind_task 的 key)     // 32 字节，AES-256
payload  = base64decode(data.bot_encrypt_secret)
iv       = payload[0:12]                            // 12 字节 nonce
tag      = payload[len(payload)-16:]                // 16 字节 GCM tag
cipher   = payload[12:len(payload)-16]
plain    = AES-256-GCM-Open(key, iv, cipher, tag)    // 无 AAD
```

`plain` 即 AppSecret。解不开或结果为空时**不要**写入设置：让运营重新生成二维码，
避免把半成品凭据存进后台。本插件只用 Go 标准库 `crypto/aes` + `crypto/cipher` 实现，无第三方依赖。

### 11.4 本插件对外映射（契约见 `schemas/bind-poll-output.json`）

| 平台 `status` | 插件 `status` | 附带凭据 |
|---|---|---|
| `0` / `1` | `pending` | 无 |
| `2` | `scanned` | `appId` + `appSecret`，**只此一次**；任务随即从内存表移除，再轮询得到 `expired` |
| `3` | `expired` | 无 |

`/bind/start` 返回的 `expiresInSeconds` 取插件侧任务 TTL（当前 10 分钟），与平台实际有效期
不必完全相等；前端以它倒计时，到点提示「重新生成二维码」。

### 11.5 明确「文档未覆盖」的部分

| 条目 | 状态 |
|---|---|
| 两个端点的频控数值 | 未覆盖（只有 `retcode`） |
| `retcode` / `msg` 的完整语义表 | 未覆盖；本插件只按 msg 关键词归类，其余给通用原因 |
| 同一机器人重复绑定的语义 | 未覆盖（是否覆盖旧凭据未说明） |
| 绑定是否影响线上机器人 / 是否触发平台审核 | 未覆盖 |
| 二维码有效期 | 未覆盖；以 `task_id` 的实际失效为准，本插件按 10 分钟兜底 |
