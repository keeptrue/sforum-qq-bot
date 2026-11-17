# NOTES · sforum.qq-bot

设计取舍、核对结论与踩过的坑。面向「下一个接手的人」，不是用户文档（那是
`README.md`）也不是接手速查（那是 `AGENTS.md`）。

## 1. 为什么独立仓库，而不是放进 SForum 主仓库

宿主主仓库已经有 `extensions/builtin/` 与 `extensions/optional/`。QQ 机器人不放进去，
理由有三条：

1. **发布节奏不同**。主仓库跟宿主版本走；QQ 机器人有自己的版本与兼容矩阵，
   不必每次宿主发版都跟着动。
2. **信任边界不同**。带后端的包要走 exact-artifact 信任确认。主仓库内置插件是宿主
   自己审过的代码，而外部仓库的包需要宿主之外的审核环节。
3. **依赖方向不同**。插件依赖宿主 SDK，反过来会让宿主依赖某个具体插件。

代价是宿主要额外扫一个「外部扩展源根」。这条能力已经存在
（`EXTERNAL_EXTENSION_ROOTS`），不新增机制。

## 2. 参考仓库的选择

骨架结构抄了两家，不抄业务内容：

- `sforum-web-search` —— 同类插件仓库（仓库根即扩展源根），它的 `.gitignore`、
  `scripts/prepare-package.sh`、`ci.yml` / `release.yml` 的骨架最贴近本仓库形态，
  直接沿用其目录约定与命名。
- `sforum-app-store` —— 它的 `ci.yml` / `release.yml` 提供了「verify → build → publish」
  三段式与品牌资产门禁的语义参照；`LICENSE` 的版权主体与它保持一致（`Inkedus`）。

## 3. 骨架阶段的 CI 行为

`backend/` 与 `sforum.extension.json` 尚未落地。若 `ci.yml` 无条件跑 `go vet ./...`，
第一次推送就会红。因此 `ci.yml` 做了**存在性判断**：

- 后端 module 不存在 → 跳过 Go 构建与测试，只发一条 `::notice` 说明尚未就位；
- 清单不存在 → 跳过 `extension digest --write` / `validate` / `test`。

这是刻意的：骨架阶段的红灯没有信息量，而绿灯 + 一条说明能真实反映「结构已就位、
业务未就位」这个状态。后端落地后，这些判断自然走真分支，不需要再改工作流。

`release.yml` **不做**这个宽容处理：它的 `verify` 在后端或清单缺失时直接
`::error` + exit 1（tag 推送与 `workflow_dispatch` 手动触发都跑）。理由是发版的
语义就是「产出可安装包」，没有包可打时失败才是正确信号；把这条门禁放宽成提示，
等于让一个打不出东西的 tag 走完全流程才失败。

## 4. 产物命名：为什么是 `sforum.qq-bot`

- **扩展 id**（清单里的 `id`、包目录名、CI 的 `PACKAGE_DIR`）：`sforum.qq-bot`。
  它必须落在 `<id>.` 命名空间下，且清单里 `id` 与包目录名一致。
- **发布产物名**（zip 文件名）：`sforum.qq-bot_<version>_<os>_<arch>.zip`。
  与扩展 id 同名，而不是仓库名 `sforum-qq-bot`。理由：用户装的是「包」不是「仓库」，
  产物名应当指向包本身；仓库名里的连字符只是 GitHub 的仓库命名约束。

## 5. 摘要与 exact-artifact

宿主按清单里的 `backend.digest` 与包内可执行文件的实际摘要做比对，对不上就拒绝启用。
这带来两个必须记住的约束：

1. 构建参数三处必须完全一致（宿主 dev watcher、`scripts/prepare-package.sh`、CI）：
   `CGO_ENABLED=0 go build -trimpath -buildvcs=false`。任何一处不同，同一份源码在两边
   产出不同字节，摘要永远不收敛。
2. **改了包内任何文件（含品牌图）都要重跑 `prepare-package.sh`**。摘要变了，旧的信任授权
   即失效，后台会显示「升级」，必须重新走一次「授权当前精确制品 → 确认并升级」。
   同版本号只改内容时，卡片上的「重启」不会切换制品——重启不是升级。

## 6. 已知要核对的宿主约束（实现阶段逐条验证）

以下是读宿主源码得到的结论，实现时需要逐条复验，不要照抄：

1. **插件设置以 `SFORUM_SETTING_<KEY>` 环境变量注入子进程**（大写、非字母数字转下划线），
   而且从未保存过的设置不会被注入——插件必须自带与清单 `default` 一致的默认值。
2. **插件子进程的环境是白名单**：`HTTP_PROXY` / `HTTPS_PROXY` 不会透传，
   出站代理只能来自插件自己的设置项。
3. **`aiTools[].contractVersion` 必须严格是 `<id>@1`**；`id` 与 `handler` 必须落在
   `<扩展 id>.` 命名空间下。
4. **Go 的 RE2 不支持 `\uXXXX` 转义**，必须写 `\x{...}`。写错的后果是
   `regexp.MustCompile` 在包初始化时 panic，插件连 go-plugin 握手都进不去。
5. **宿主 `DigestTree` 禁止符号链接**：不要用软链把包目录挂到别处。

## 7. 品牌资产

`brand/` 下的三张图由设计师产出，规格沿用宿主约定：

| 文件 | 尺寸 | 上限 |
| --- | --- | --- |
| `brand/icon.svg` | `viewBox="0 0 512 512"`，自包含 | 40 KiB |
| `brand/icon.png` | 512×512 PNG | 120 KiB |
| `brand/social-preview.png` | 1280×640 PNG | 600 KiB |

三张图**由本仓库自行绘制并导出**，不走宿主 `SForum/scripts/brand/` 下的生成器模板：
那套模板把主色写死为 SForum 主蓝 `#2671ed`（`BRAND_START`），且
`SForum/scripts/brand/spec.json` 里没有 `sforum.qq-bot` 条目（`grep -c 'qq-bot'` → 0），
照走会用主蓝覆盖本插件的 QQ 蓝。矢量源是 `brand/icon.svg` 与
`docs/brand/social-preview.svg`，导出命令见 `plugins/sforum.qq-bot/README.md`
的「重新导出」段（headless chrome 截图 + Pillow 压缩）。

校验走宿主门禁：`go run ./cmd/sforum extension brand check <包路径>`，已串进
`scripts/prepare-package.sh` 与 `release.yml`。

品牌文件不进 `packageFiles`，但属于包树：改图会改变包摘要，走正常升级流程。
**不要为了消掉摘要漂移去改版本号。**

## 8. 凭据边界

QQ 开放平台的 AppID / AppSecret 只走后台设置页（`secret` 类型），由宿主以
`SFORUM_SETTING_*` 注入子进程。以下位置一律不放凭据：

- 仓库文件（含 `NOTES.md`、示例配置）；
- 扩展清单 `sforum.extension.json`；
- 提交进 git 的 `.env`。

理由很直接：清单与安装包是公开可下载的，把凭据写进去等于把它发出去。

## 9. 待验证的问题（实现阶段）

1. **待办：把本插件纳入宿主的品牌生成器流程，或正式记录为「例外」。**
   宿主 `SForum/scripts/brand/` 的生成器把主色写死为 SForum 主蓝 `#2671ed`，
   `spec.json` 里也没有 `sforum.qq-bot` 条目（`grep -c 'qq-bot'` → 0）。当前做法是
   本仓库自行绘制（见 §7），代价是**宿主改了品牌模板，本插件不会跟着更新**。
   两条出路：给 `spec.json` 补一条 `sforum.qq-bot` 条目并允许覆盖主色；或在宿主侧
   记一份「插件自有品牌例外」清单。两件事都不该由插件单方面决定，需与宿主侧对齐。
2. QQ 侧长连接（WebSocket）与 HTTP 回调两种接入方式，哪种更适合长驻的论坛实例？
   倾向长连接：省掉公网回调地址的部署成本，但需要处理断线重连与多实例抢占。
3. 多个论坛实例同时指向同一个 QQ App 时，事件会被多份消费。是否需要在插件侧做
   租约，还是交由运营者保证一个 App 只对一个站点？
4. 入站回流是否应该默认关闭。默认开启意味着任何群成员发言都可能变成论坛内容，
   这在权限模型上是危险的。倾向默认关闭 + 显式关键词前缀 + 显式群白名单。

## 10. M1.1 设计取舍与查证（扫码绑定 + 组件设置页）

这一节记录 M1.1 的三个决定（扫码必须做组件、协议从哪来、契约为什么长这样）以及
它们各自的**宿主源码依据**。行号是本轮（2026-11-18）在 `../SForum` 检出上核对的位置；
宿主改版后以语义为准、行号会漂。

### 10.1 为什么不能把二维码塞进「探测结果」

最初的想法是：设置页加一个 `ai_tool_probe` 按钮，插件返回 ASCII 二维码，运营照着扫。
两条宿主事实直接否掉了这条路：

1. **探测结果只是一行纯文本**。宿主渲染探测返回值的地方是
   `apps/web/app/components/extensions/settings/SFExtensionSettingsActions.vue:35-41`：
   一个 `UAlert`，`:title` 取 `message || reason`、`:description` 取 `suggestions.join(' ')`。
   没有 `<pre>`、不渲染图片、HTML 空白折叠——多行 ASCII 二维码会被压成一行乱码。
2. **探测 runtime 的临时目录是 per-process 私有的**。
   `apps/api/app/Support/Extensions/pluginprocess/sandbox_tempdir_linux.go:107`：
   `dir := filepath.Join(paths.Root, fmt.Sprintf("%s-%d", sandboxNameSuffix(spec.PluginID), os.Getpid()))`
   ——目录名里带 pid；`:116` 的注释还说明 exec 后 pid 不变、owner 记录与目录一起落地。
   两次探测是两次独立进程，**不能**靠临时文件在 `start` 与 `poll` 之间传绑定任务；
   而把整个绑定流程塞进一次探测又必然撞上 15 秒的探测总预算。

结论：扫码必须由**设置页预构建组件**（浏览器里轮询）+ **插件自己的 HTTP 路由**（进程内状态）共同完成。
AI 工具探测只保留给「预览 / 测试 / 状态」这类一次性读。

### 10.2 协议来源与实现边界

- 端点、字段与二维码 URL 形态来自腾讯官方 npm 包 `@tencent-connect/qqbot-connector@1.2.0`。
  **官方 wiki 没有覆盖这两个接口**，因此 `docs/qq-api-v2-capabilities.md` 单独做了附录，
  并标注「平台可能变更」。
- Go 侧**零新增依赖**：AES-256-GCM 解密只用标准库 `crypto/aes` + `crypto/cipher`
  （`backend/bind.go`：key = base64 解码的 32 字节，payload = base64 解码的密文，
  前 12 字节 IV、末 16 字节 tag）。
- 二维码在前端用 `uqr` 画成 SVG（`frontend/admin/src/bind-card.ts`），
  Go 侧不引二维码库，也不生成图片。
- 绑定任务只在内存里（`backend/bindstore.go`，TTL 10 分钟、有上限与限流），
  进程重启即失效——这是刻意的：任务里带着解密 key，不该落盘。
- 凭据只在 `completed` 那一次轮询里返回一次：`routeBindPoll` 取走即删，后续轮询看到 `expired`。
  AppSecret 只写进宿主草稿（`setValue('app_secret', …, false)` 不写 input），保存后宿主掩码回空。

### 10.3 组件模式对契约的影响

- `settings.ui.mode = "component"` 时组件**整体替换** schema 表单
  （`apps/web/app/pages/admin/extensions/[extensionId]/pages/[...pagePath].vue:383` 挂组件、`:407` 走表单），
  所以 `fields` 必须保留且能独立工作：它是信任未批/组件加载失败时的回退。
- 未授权时组件内部渲染「授权面板 + `#fallback` 插槽里的表单」
  （`apps/web/app/components/extensions/settings/SFTrustedSettingsComponent.vue:305-312`），
  运营看到的是「授权提示 + 完整表单」，功能不丢。
- 组件模式下宿主**不渲染** `actions[]`，所以「测试推送 / 状态 / 预览」必须另走插件自己的
  `routes[]`；同一批能力在 schema 回退路径上继续由 `ai_tool_probe` 提供（清单里三个 action
  指向三个 AI 工具），两条路径共用同一份渲染与状态实现。
- 路由授权：`guard: core.guard.permission` + `permission: sforum.qq-bot.manage`。
  permission 为空即不可经 legacy 前缀代理
  （`apps/api/app/Support/ExtensionManifest/route_access.go:18-77`），且该键必须在
  `permissionDefinitions` 里声明（`v3_validate.go:153`）。`assignmentPolicy` 只能是 `"host"`
  （`v3_validate_platform.go:705`），漏了清单直接 invalid。

### 10.4 openapi 片段为什么是必需的

`extension validate` 会为声明了 `routes[]` 的包构建路由 schema 目录
（`apps/api/cmd/sforum/validate.go:97` → `preflightRouteSchemaCatalog`）。构建时：

- 引用**不以 `.json` 结尾**（也就是契约式引用 `…@1`）时，只能从 OpenAPI 片段的
  `components.schemas` 里取（`apps/api/app/Support/ExtensionOpenAPI/route_schema_catalog.go:525` 起：
  非 `.json` 走 shared definitions，取不到就 `ErrRouteSchemaMissing`）；
- 一旦声明了片段，**每条 addressable route 都必须出现在片段里**
  （`apps/api/app/Support/ExtensionOpenAPI/aggregate.go:148`：`route … is missing from OpenAPI fragments`）。

所以 `openapi/routes.yaml` 是契约的一部分，不是文档装饰：它把 5 条路由声明成 OpenAPI operation，
再用 `components.schemas` 里的 `$ref` 指向 `schemas/*.json`。**schema 内容只有一份**（那 10 个 JSON），
片段只负责「契约引用 → 文件」的绑定。删掉片段后 `validate` 会直接报
`route schema catalog: … route schema is missing`（本轮已用反证实验确认）。

### 10.5 前端产物：入库、可复现、node_modules 必须删

- 宿主只读 `frontend/admin/dist`，**不编译源码**，所以 dist 必须随源码提交；
  CI 会 `bun run build` 后 `git diff --exit-code` 比对，漂移即失败。
- `node_modules` 必须在算摘要前**物理删除**：宿主算作者树摘要走 `DigestAuthorTree`（跳过它），
  exact-artifact 校验走 `VerifiedTreeDigest`（不跳过），同一棵树上两边必然不一致，
  报 `package digest mismatch after static inspection`。`prepare-package.sh` 与两个工作流都这么做。

### 10.6 与任务书的偏差（实现事实，已回填到 plan 的 §13）

| 任务书条目 | 实际实现 | 原因 |
| --- | --- | --- |
| F-14 合并窗口 60 秒 | 去重窗口 1 小时（`backend/dispatcher.go:16`，键 = deliveryId + topicId，命中记 `duplicate`） | 比 60 秒更严，覆盖「同一主题被重复投递」的主要场景；窗口可后续调小 |
| F-15 日配额「触达即降级为摘要」 | 触顶后当日不再发送，投递记录写 `quota_exceeded`（`backend/dispatcher.go:183` 起 `deliver` → `allowDaily` 分支） | 摘要降级要再拉一次正文且收益不明显；停发更可预测，状态页能直接看到「触顶」这条记录 |
| F-16 投递记录可导出 | 内存环 50 条（`backend/state.go:8`），状态页展示最近 10 条，无 CSV（F-33 P1 未做） | M1 只承诺「可查」；落盘与导出涉及宿主存储面，留到后续 |
| F-13 事件开关清单 | 只订阅 `topic.created`，没有逐事件开关 | 本期只放行新主题（裁决 D3），开关本身没有第二个选项可切 |
| F-04 接入模式选择 | 只实现长连接（用于群 ID 识别），HTTP 回调未做 | 群 ID 识别必须长连接在线；回调要公网入口，本期不需要 |
| F-03 调试/正式目标分账 | 未实现 | 官方新版管理端已无沙箱环境概念，运营标签没有落点 |
| F-30 审计 | 只到插件自己的投递记录，未接宿主审计 | 宿主审计面覆盖推送动作需要额外契约，M1 不做 |
