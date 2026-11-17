# 变更记录

`sforum.qq-bot` 的对外变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

发布说明由 `.github/workflows/release.yml` 从对应版本的小节生成，因此**打标签前必须先写这一节**，
否则 Release 只会有安装步骤。

## [Unreleased]

尚未发布的改动记在这里。

## [0.4.0] - 未发布

M1.3：补上入站的最小闭环（被 @ 回一句）与设置页顶部的账号身份。路由 6 → 7，
包内文件 18 → 20，设置项 16 → 17。

### 新增

- **被 @ 时回一句说明**：群里 @ 机器人时，用事件里的 `d.id` 作 `msg_id` 发一条**被动回复**
  （`msg_seq` 固定 1），内容是「本机器人只做 SForum 论坛新帖推送；这个群已记录，可在后台勾选为
  推送目标。」。被动回复不占主动消息频控。边界按官方口径：`msg_id` 5 分钟内有效、每条消息最多被
  回复 5 次；插件自己再加一道「同群 60 秒只回一次」的闸（进程内，重启即清空）。`msg_seq` 固定是
  故意的——自增序号只会绕开平台的重复保护，让「回过了」变成「再回一次」。
- **回复失败不影响群 ID 认领**：认领先做、回复后做，回复失败只记一行日志。拿不到 `d.id` 或
  `group_openid`、以及 `author.bot` 为真的消息一律静默跳过。
- **新设置项 `reply_on_mention`**（boolean，默认开，分组「推送目标」）：关掉后只记录群 ID、不回复。
- **`POST /account/status`**（第 7 条路由，权限 `sforum.qq-bot.manage`）：一次返回三块——`bot`
  （`GET /users/@me` 的昵称与头像，缓存 10 分钟、失败缓存 1 分钟；未配凭据 / 401 / 网络失败一律
  `available=false` + 可读原因，路由仍 200）、`gateway`（只读进程内连接状态与 reason，**不发任何
  请求**）、`groups`（已识别群与白名单去重条数，与设置页「目标群 N 个」同口径），另带 `appId`、
  `credentialsConfigured` 与 `checkedAt`。
- **设置页「当前账号」卡片**：页面顶部显示头像 / 昵称 / AppID、五种连接状态徽标（已连接 / 连接中 /
  未启用 / 授权失败 / 连接异常；未配凭据时改显「尚未配置凭据」）、「已识别群 N · 白名单 M」
  （白名单非空时可点，跳到群 ID 列表）。挂载即拉、手动刷新、保存后自动刷新；30 秒轮询，页面隐藏
  时暂停、可见时立刻补一次。

### 变更

- **错误码翻译补 5 条**：`40034005` / `304103` → 「被动回复窗口已过期（msg_id 超过 5 分钟或已被
  使用）」；`40054005` → 「该消息已被回复过（msg_id 与 msg_seq 重复）」；`40034101` / `40054003`
  → 「机器人不在该群里（可能已被移出），请重新拉入后再试」。`40034128` 的文案改为同时说明两种
  语境：「主动消息受限，或被动回复的时间/次数已用尽」——同一个码在主动与被动两侧都会出现。
- 清单：`routes` 6 → 7（新增 `account_status`）、`packageFiles` 18 → 20（新增
  `schemas/account-status-{input,output}.json`）、设置项 16 → 17（新增 `reply_on_mention`）、
  `version` `0.3.0` → `0.4.0`；`openapi/routes.yaml` 的 `info.version` 同步。

### 文档

- `NOTES.md` 新增 §12：被动回复与主动消息频控的官方口径（含 `GROUP_ADD_ROBOT` 可用 `event_id`
  回复、群侧没有「列出机器人所在群」接口两条结论）。
- `AGENTS.md` / `README.md` 的「当前状态」同步到 `0.4.0`。

## [0.3.0] - 未发布

M1.2 加固：修掉 0.2.0 真机验收前暴露的四类风险，并补两项运营直接用的功能。
路由 5 → 6，包内文件 16 → 18。

### 新增

- **已识别群列表 `POST /groups/list`**：把插件自己收到的群事件（`group_openid` / 事件类型 /
  首次与最近时间 / 次数）回给设置页，另附 `claimEnabled` 与 `recordingLimit`（环形容量 20）。
  QQ 群侧没有「列出机器人所在群」的接口，数据源就是这些事件，不读宿主数据库。
- **群名尽力而为**：`GET /v2/groups/{openid}/info` 取不到只让该群 `name=null` +
  `nameUnavailable=true`，绝不让整个列表失败。约束：单次最多查 10 个、并发 3、整次请求
  4 秒预算、进程内共享 30 QPM 令牌桶；结果进缓存（成功 1 小时，失败按性质分档，见「修复」）。
- **设置页「已识别的群」勾选列表**：白名单文本域上方列出识别到的群，勾选/取消直接增删
  `group_ids` 行；文本域仍是唯一真源，手写行与 `#` 注释原样保留。群名不可用时回落
  `openid` 前 6 位 + `…`；识别开关关闭（`claimEnabled=false`）与路由不存在各有降级文案。
- **模板预设**：默认 / 精简 / 公告风三个按钮把模板写进草稿并复用既有预览；「默认」读清单里的
  `item.default`，不在组件内复制一份文案。预设不落库，点「保存」才生效。
- **`/preview` 支持 `source=saved|draft` 与 `sample`**：`saved`（默认）忽略请求里的模板类字段、
  完全按已保存配置渲染，与真实推送共用同一个 `renderSaved` 入口，并在响应里回显
  `savedSnapshot`（`message_template` / `message_format` / `signature` / `excerpt_limit` /
  `max_message_chars`，全部取自服务端配置、不取自请求；`draft` 刻意不带，避免草稿被当成基线）；
  `draft` 保留编辑期预览（缺键回落已保存、空串按空串）；`sample` 可按字段覆盖样例的
  标题/作者/分类/摘要/链接。响应新增 `source` / `characters`（按码点计数）/ `sampleUsed` /
  `savedSnapshot`。
- **预览卡的「样例数据」覆盖**：预览区新增可折叠面板，五个样例字段（标题 / 作者 / 分类 /
  摘要 / 链接）只把填了的发出去，其余仍用内置样例；`sampleUsed` 为真时给一行弱提示，
  说明当前预览用的是自定义样例而不是真实主题。

### 修复

- **`/bind/poll` 只认绑定任务创建者**：任务表增加 `ownerID`，非创建者取 Key / Take 返回 false
  且**不删条目**（任务仍归发起者）；`/bind/start` 取不到账号直接 403，避免留下
  `ownerID=0`、谁都能取走的任务。非创建者与「任务不存在」共用同一条出口
  （`200` + `status=expired`），不向非本人确认 taskId 是否存在。
- **预览区分「缺键」与「空串」**：`template` / `signature` 改为 presence-aware——键在就按给的
  值渲染（空串渲染空），缺键才回落已保存配置。此前清空模板后预览仍显示已保存模板的渲染结果，
  前端「模板为空」提示几乎不可达。`maxMessageChars` 的「0 = 沿用默认」与测试消息的
  「空串 = 沿用已保存值」是另外两条约定，未改动。
- **群名查询的等锁可取消、失败缓存分档**：`refreshMu` 换成容量 1 的 channel 信号量 +
  `select ctx.Done()`，取消只发生在**等锁**阶段、不会留下半刷新的缓存；失败缓存由一刀切
  10 分钟改为 11253（确定性无权限）与业务错误 10 分钟、瞬时失败（传输层 / 超时 / 取消 /
  5xx）45 秒。

### 变更

- **设置页预览默认按已保存配置渲染**（即真实推送口径）：草稿有未保存改动时显示提示行与
  「按草稿预览」切换按钮，保存后自动切回 `saved`。**预览基线改用 `/preview source=saved`
  回显的 `savedSnapshot`**：宿主 `settings.values()` 恒等于草稿、`settings.items` 只是挂载时
  快照，且 `save()` 失败并不 reject，组件无法分辨保存成败——旧做法据此推进基线，保存失败时
  会误报「已保存」。现在保存/恢复失败基线不推进，脏提示保留；`excerpt_limit` /
  `max_message_chars` 在基线里统一归一为字符串，避免「语义相同、类型不同」被判成脏。
- **删 `state.Snapshot` 与 `(*State).Snapshot()`**：全仓库零调用，且其注释声称「返回脱敏副本」
  与事实相反——真实出口 `RecentClaims` **故意**返回完整群 ID（运营要复制进白名单），
  插件里从来没有脱敏的状态出口。
- 清单：`routes` 5 → 6（新增 `groups_list`）、`packageFiles` 16 → 18（新增
  `schemas/groups-list-{input,output}.json`）、`version` `0.2.0` → `0.3.0`；
  `openapi/routes.yaml` 的 `info.version` 同步。

### 文档

- `AGENTS.md` / `README.md` 的「当前状态」同步到 `0.3.0`。
- **真机验收仍待 YG 完成**：`docs/acceptance-0.2.0.md` 的四步（扫码绑定 / 群 ID / 测试消息 /
  真帖推送）尚未打勾，0.3.0 新增的群列表勾选与预览双模式也需要补测。

## [0.2.0] - 未发布

M1 推送闭环 + M1.1 扫码绑定与组件设置页。首个可安装版本。

### 新增

- **新帖推送闭环**：观察 `topic.created`，按模板渲染后推送到白名单群；摘要走 HostQuery
  `sforum.core.public_topic.by_id`（不读宿主数据库），正文取不到时按空摘要继续，不阻断推送。
- **手机 QQ 扫码绑定**：插件路由 `/bind/start` 生成绑定任务与二维码链接、`/bind/poll` 轮询进度；
  完成时用 AES-256-GCM（标准库，零新增依赖）解密平台下发的 AppID / AppSecret，
  设置页自动写回并提交保存。AppSecret 只交付一次，不落日志、不进状态文本。
- **设置页预构建组件**（Vite + TypeScript，产物 `frontend/admin/dist` 入库）：扫码卡片、
  模板变量点选插入、随改随显示的实时预览、底部预览 / 测试 / 状态与保存、恢复默认。
  信任未批或组件加载失败时，宿主回退到声明式表单，功能不丢。
- **插件 HTTP 路由 5 条**：`/bind/start`、`/bind/poll`、`/preview`、`/test-push`、`/status`，
  统一 `guard: core.guard.permission` + 权限 `sforum.qq-bot.manage`（清单 `permissionDefinitions` 声明）。
- **契约文件**：10 个 JSON Schema（5 组 input/output）+ `openapi/routes.yaml` 路由片段
  （把 `…@1` 契约引用绑到 schema 文件，`extension validate` 的路由 schema 目录依赖它）。
- **群 ID 自动识别**：长连接订阅群事件，把 `GROUP_ADD_ROBOT` / `GROUP_AT_MESSAGE_CREATE`
  看到的 `group_openid` 记进状态，「推送状态」里可直接复制。
- **模板能力**：`{{title}} {{author}} {{category}} {{tags}} {{excerpt}} {{url}} {{topicId}} {{date}} {{signature}}`
  与中文别名；`text` / `markdown` 两种形态；超长截断、未知变量原样保留并回报。
- **推送护栏**：每分钟令牌桶、每群每日上限（默认 800，平台侧 1000）、有界重试与退避、
  全局暂停开关、队列拥塞时丢最旧一条。
- **错误可读化**：把 QQ 平台错误码翻译成管理员可读的一句话（凭据无效、主动消息受限、
  频控、消息过长、需要平台白名单等），只保留必要标识，不带请求体与凭据。

### 变更

- 扩展清单从骨架补全为可安装形态：`events` / `routes` / `permissionDefinitions` /
  `aiTools`（3 个）/ `openapi` / `packageFiles`（16 项）；版本 `0.1.0` → `0.2.0`。
- 设置页从纯 schema 表单改为「预构建组件 + 表单回退」；`ui.mode` 由 `schema` 改为 `component`，
  底部动作由 2 个（测试、状态）增加到 3 个（预览、测试、状态），文案同步为三个按钮。
- 前端产物改为**随源码入库**：宿主不编译前端源码，CI 与发布流程都会重建 dist 并与提交内容比对。
- CI 的 `verify` 新增前端构建与 dist 一致性检查（漂移即失败）；发布流程新增前端三件齐备检查。

### 文档

- `README.md` 重写：三分钟接入（含扫码点击路径与两个前置：插件启用、组件信任授权）、
  手动填写凭据、QQ 侧三个前置条件、常见错误对照表、真实设置项表（16 项）。
- `NOTES.md` 新增 §10「M1.1 设计取舍与查证」：单行探测结果与 per-process 临时目录的宿主源码依据、
  扫码协议来源、openapi 片段必需的原因、与任务书的实现偏差清单。
- `docs/qq-api-v2-capabilities.md` 新增 §11「扫码绑定（q.qq.com lite）」：两个端点、字段、
  状态枚举、AES-256-GCM 参数、二维码 URL 形态，并标注来源为官方 npm SDK 且 wiki 未覆盖。
- `docs/plan-qq-bot.md` 回填 M1 完成项、M1.1 增量与交付状态（§6、§13）。
- `AGENTS.md` 更新当前状态、目录树（含 `frontend/admin/`、`openapi/`、`schemas/`）、
  命令（前端构建、dist 校验、打包全链路）与硬边界（dist 必须入库、`node_modules` 必须物理删除）。

## [0.1.0] - 未发布

首个版本（M0 仓库骨架阶段，尚未发布）。

### 新增

- 独立插件仓库骨架：仓库根即 SForum 的外部扩展源根，扩展包位于 `plugins/sforum.qq-bot/`。
- 根级约定与文档：`AGENTS.md`（仓库级 AI 约定）、`README.md`（运营者说明）、
  `NOTES.md`（决策与踩坑记录）、`CHANGELOG.md`、`LICENSE`（MIT）。
- `scripts/prepare-package.sh`：本机构建二进制 → 刷新包摘要 → 清单校验 → 品牌校验 → 宿主契约检查。
- `.github/workflows/ci.yml`：push `main` / PR / 手动触发，后端 vet/test、宿主平台构建、
  发布矩阵交叉编译冒烟、宿主契约校验；后端尚未就位时只做结构检查并给出提示。
- `.github/workflows/release.yml`：`v*` 标签触发，四平台交叉编译产出
  `sforum.qq-bot_<version>_<os>_<arch>.zip` 并挂到 Release，附 `SHA256SUMS`。
- 扩展包骨架目录：`plugins/sforum.qq-bot/` 下的 `manifest/langs/`、`backend/`、`brand/`。

### 说明

- 品牌三图（`icon.svg` / `icon.png` / `social-preview.png`）已入库；
  扩展清单与插件后端在本版本尚未落地，因此当时没有可安装的扩展包。
