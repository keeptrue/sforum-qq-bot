# NOTES · sforum.qq-bot

设计取舍、核对结论与踩过的坑。面向「下一个接手的人」，不是用户文档（那是
`README.md`）也不是接手速查（那是 `AGENTS.md`）。

> 后台反复提示「重新授权」、`backend.digest` 在两个值之间来回跳？先看 §11：
> 那是宿主 dev 构建与本仓库发布构建的口径差异，不是 bug。

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

1. **编译参数**在本仓库三者之间必须一致（`scripts/prepare-package.sh`、`ci.yml`、`release.yml`）：
   `CGO_ENABLED=0 go build -trimpath -buildvcs=false`，不带 `-ldflags`。但**摘要不跨环境相同**——
   宿主 dev 构建带 `-ldflags=-s -w`，两个工作流又把 replace 重写成绝对路径，而本地脚本不钉
   `GOTOOLCHAIN`（跟着本机 go1.27.1 跑，CI 按宿主 `go.mod` 钉在 go1.26.11）。dev/CI 与本地
   制品摘要天然不同，属预期而非缺陷，三个变量的实测见 §11.3，处置与判据见 §11.4 / §11.5。
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

## 11. dev 制品摘要与 release 摘要天然不同（不要对齐，也别每轮当 bug 重查）

### 11.1 现象

后台里同一个插件反复出现「重启 → 需要授权 → 确认升级 → 又要重新授权」，
`sforum.extension.json` 的 `backend.digest` 在两个值之间来回跳。看起来像摘要算错了。

**它不是。** 同一份源码被两个构建者用不同的命令行各构建了一次：宿主 dev watcher
在开发机上重建 `backend/plugin` 并回写清单摘要，我们的脚本与流水线按发布口径再建一次。
字节不同 → 摘要不同 → 信任授权随制品失效。这是设计后果，不是缺陷。

### 11.2 根因：三处构建口径差三个变量

三处（宿主 dev watcher / `scripts/prepare-package.sh` / `.github/workflows/ci.yml`，发布工作流与
ci 同口径）都带 `CGO_ENABLED=0`、`-trimpath`、`-buildvcs=false`。差别在三个变量上：
`-ldflags`、replace 路径是否进 buildinfo、**Go 工具链版本**。下表的命令行逐字抄自对应文件
（行号是 2026-11-18 在本机两份检出上核对的位置，宿主改版后会漂）：

| 构建者 | 出处 | 实际命令行 | `-ldflags=-s -w` | replace 路径进 buildinfo | Go 工具链 |
| --- | --- | --- | --- | --- | --- |
| 宿主 dev watcher | `SForum/apps/api/app/Support/ExtensionPackage/dev_build.go:85`（`backendBuildArgs`；环境由 `:121` 的 `newBackendBuildPlan` 给出） | `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-s -w -o backend/plugin.tmp ./` | **有** | 无（模块里有文件系统 replace 时改用临时 go.work） | `GOTOOLCHAIN=go1.26.11`，取自**插件** `backend/go.mod` 的 `go` 行 |
| `scripts/prepare-package.sh:67` | | `(cd "$PKG/backend" && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$PKG/backend/plugin" ./)` | **无** | 有（相对路径 `=> ../../../../SForum/apps/api`） | **不钉**，用本机默认（本轮 `go version` = go1.27.1） |
| `.github/workflows/ci.yml:179` | | `run: CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$GITHUB_WORKSPACE/plugin/${PACKAGE_DIR}/backend/plugin" ./` | **无** | 有（绝对路径，`:164` 的 `go mod edit -replace` 重写） | `go-version-file: sforum/apps/api/go.mod`（`:80`）→ 宿主 `go 1.26.11` |

发布工作流与 ci.yml 同口径（没有 `-ldflags`，同样重写 replace，同样读 `go-version-file`），
差别只在 GOOS/GOARCH：`release.yml:230-232`（`go build` 在 `:232`；`go-version-file` 在 `:51`
与 `:176`，`go mod edit -replace` 在 `:121` 与 `:181`）。

`scripts/prepare-package.sh`、`ci.yml`、`release.yml` 里那**三处注释**在 2026-11-18 已按事实改正
（此前它们都把「三处必须一致」偷换成「与宿主 dev 一致、一律不加 `-ldflags`」，会把后来人教坏）：
现在写的是「不带 `-ldflags` 是发行口径、摘要不跨环境相同」。它们只点出了前两个变量，本节表格
补上第三个：工具链。历史成因备查：宿主后来才补上两件事——`-ldflags=-s -w` 来自 `42c2a4483`
（dev watcher 与 build.sh 产物对齐），`GOTOOLCHAIN` 钉到 `go.mod` 的 `go` 行 + 临时 go.work
剔掉 replace 的文件系统路径来自 `1906f971f`。**本仓库三者之间**现在真正一致的只有**编译参数**
（`CGO_ENABLED` / `-trimpath` / `-buildvcs=false` / 不带 `-ldflags`）；宿主 dev 连这一组都多一个
`-ldflags`。摘要从来不是跨环境可对齐的量。

**replace 路径与工具链两个变量都在改字节。** 两个工作流都把 replace 重写成绝对路径
（`ci.yml:164`、`release.yml:121` 与 `:181` 的 `go mod edit -replace …="$GITHUB_WORKSPACE/sforum/apps/api"`），
而 `prepare-package.sh` 保持 `go.mod` 里的相对路径；replace 目标会逐字进 buildinfo，`-trimpath`
不处理它。工具链则更直接：`prepare-package.sh` 不钉 `GOTOOLCHAIN`，本机是 go1.27.1，而两个工作流
按宿主 `go.mod` 钉在 go1.26.11（插件 `backend/go.mod` 的 `go` 行同样是 1.26.11，所以宿主 dev 与 CI
在这一点上一致，只有本地脚本跟着机器跑）。所以**CI 算出的摘要天然不等于本地 `prepare-package.sh`
的摘要**，宿主 dev 的摘要又天然不等于两者——这正是 `ci.yml:198-203` 那段「摘要与提交值不一致只发
notice」存在的原因：不是构建命令写错，是这三个变量都在改字节，而 CI 钉的是宿主的 1.26.11。

**结论：制品摘要由三个变量共同决定——`-ldflags`、replace 路径是否进 buildinfo、Go 工具链版本。**
任意一个变了，摘要就整条换掉（§11.3 逐行实测）。所以「同一份源码应该算出同一个摘要」这个直觉
只在一台机器、一条命令、一个工具链下成立；跨环境对齐要三个变量一起按住，值不值见 §11.4。

### 11.3 差额实测：同一份源码的五个摘要（2026-11-18，linux/amd64）

五个值都在本机测得，源码完全相同，每次只动一个变量。第 1 行是入库值，第 3 行是 CI 口径，
第 4 行是宿主 dev 口径，第 2、5 行是受控对照：

| 构建口径 | Go 工具链 | replace 形态 | `-ldflags` | 体积 | sha256 | buildinfo 里的 replace |
| --- | --- | --- | --- | --- | --- | --- |
| 1. `prepare-package.sh`＝**当前入库值** | go1.27.1 | 相对 | 无 | 25,554,478 B | `f2b92b31218cf34f95fd68e838d3cdc717b18eb63cc406158892561a9b94be49` | `=> ../../../../SForum/apps/api` |
| 2. 对照：只换工具链（其余同第 1 行） | **go1.26.11** | 相对 | 无 | 24,508,499 B | `8bf8b800195dc7503479a63d3cdbc06be471fde921ace7b737367e4301559d6b` | `=> ../../../../SForum/apps/api` |
| 3. 两个工作流（CI / release 口径） | go1.26.11 | **绝对** | 无 | 24,508,499 B | `b3c6561d2924f2f785d43925216fca168f0a091d5bd53f996a762df8ea0b4176` | `=> /abs/…/SForum/apps/api` |
| 4. 宿主 dev（go.work 无 replace 行） | go1.26.11 | 无（临时 go.work） | **`-s -w`** | 17,039,522 B | `eb1118c38ab4adb830f84ce1c318bdd52569052fdbe3df2e42c44ecaf7414390` | 整行缺失 |
| 5. 对照：只加 `-ldflags=-s -w`（其余同第 3 行） | go1.26.11 | 绝对 | `-s -w` | 17,039,522 B | `8d21214330d9c87db61865a71375b1d961367cf3273e895d94c3925e76b335b4` | `=> /abs/…/SForum/apps/api` |

读法：

- **1 → 2 只换工具链**：摘要整条换掉，体积差 1,045,979 B。工具链不是「同一份代码换个版本也一样」的量。
- **2 → 3 只换 replace 写法**：体积一模一样（24,508,499 B），摘要却不同——路径字符串只改 buildinfo
  里的字节，不动代码段大小。
- **3 → 5 只加 `-ldflags`**：体积从 24,508,499 B 掉到 17,039,522 B（少 7,468,977 B）；摘要同样整条换掉。
- **4 与 5** 体积相同、摘要不同：第 4 行没有 replace 行，第 5 行有绝对路径那行——再次证明
  「体积相同 ≠ 字节相同」。

**replace 路径确实进 buildinfo，`-trimpath` 不处理它；工具链也逐字节体现在产物里。** 复现
（三条都验过，全部不写包内文件）：

```bash
# 1) 入库二进制是谁写的：首行给工具链，grep 给 replace 形态
go version -m plugins/sforum.qq-bot/backend/plugin | head -1
#    plugin: go1.27.1                            ← 本机工具链（脚本不钉 GOTOOLCHAIN）
go version -m plugins/sforum.qq-bot/backend/plugin | grep -F '=>'
#    相对路径  => ../../../../SForum/apps/api   ← prepare-package.sh 的产物
#    绝对路径  => /home/…/SForum/apps/api       ← 两个工作流的产物
#    整行缺失                                   ← 宿主 dev 的产物（走临时 go.work）

# 2) 只换工具链：原地构建、产物写 /tmp（相对 replace 只有在原位才解析得开）
(cd plugins/sforum.qq-bot/backend && GOTOOLCHAIN=go1.26.11 CGO_ENABLED=0 \
   go build -trimpath -buildvcs=false -o /tmp/plugin-g126 ./ && sha256sum /tmp/plugin-g126)
#    期望 8bf8b800…（24,508,499 B），而不是入库的 f2b92b31…

# 3) 再把 replace 改成绝对路径（等于两个工作流的做法），摘要又变
cp -a plugins/sforum.qq-bot/backend /tmp/qqbot-abs
sed -i "s#^replace github.com/keeptrue/sforum/apps/api v0.0.0 => .*#replace github.com/keeptrue/sforum/apps/api v0.0.0 => $(cd .. && pwd)/SForum/apps/api#" /tmp/qqbot-abs/go.mod
(cd /tmp/qqbot-abs && GOTOOLCHAIN=go1.26.11 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o plugin ./ && sha256sum plugin)
#    期望 b3c6561d…（CI 口径）；再加 -ldflags="-s -w" 得到 8d212143…（第 5 行）
```

注意第 2 步为什么必须**原地**构建：`go.mod` 里是相对 replace（`../../../../SForum/apps/api`），
把它整目录 `cp` 到 `/tmp` 之后相对路径就指不到了，构建会直接报
`reading …/SForum/apps/api/go.mod: no such file or directory`。要么原地构建、产物外置，
要么像第 3 步那样先改成绝对路径再搬。

反向也验过：第 1 行那条命令在本机可逐字节复现（`f2b92b31…be49`，25,554,478 B），说明差异来自
构建口径，不来自源码。

### 11.4 正确处置：不对齐，也不改宿主

**决策（YG，2026-11-18）：不改宿主仓库。** 两边约定的是「同一棵插件树由两个写者共同维护」，
而不是「同一个二进制」。落地成三条**处置**（这里说的是怎么做，不是 §11.2 那三处文件注释）：

1. 看到摘要不一致、后台要重新授权，先按 §11.5 分类；**不要**去调参数「对齐」。
2. 真要两边逐字节一致，得把 §11.2 那三个变量**同时**按住，两条路都必须闭环：① 让本仓库靠向
   宿主——脚本与流水线补 `-ldflags=-s -w`、把 `GOTOOLCHAIN` 钉到 1.26.11、再复刻临时 go.work
   剔掉 replace 路径的逻辑（等于完全复刻宿主 dev 口径）；② 让宿主靠向本仓库——去掉 `-ldflags`、
   改回相对 replace，**并且**不再按 `go.mod` 钉 `GOTOOLCHAIN`（`dev_build.go:127`）。②少了最后
   一条就不闭环：宿主仍用 1.26.11、本地仍是 1.27.1，摘要会停在 §11.3 第 2 行的 `8bf8b800…`，
   与入库的 `f2b92b31…` 依然不等。两条都是改发布口径 / 改宿主，代价远大于收益——dev 产物只在
   开发机上有效，交付与发版一律以 `release.yml` 构建的制品为准。
3. 验收与发版判断只认 `scripts/prepare-package.sh` 重跑后的摘要。宿主 dev 构建覆盖出来的
   那一份不进入任何交付结论。

### 11.5 判据：一眼分清「正常 dev/release 差异」和「改了内容忘了重跑」

1. **`git status --short plugins/` 输出为空** → 包树没被动过，摘要不一致只能来自构建口径，
   是正常的 dev/release 差异，别管它。
2. **输出非空**，或 `git diff` 里 `backend.digest` / `packageFiles[].digest` 变了却没有对应
   内容改动 → 改了包内文件而没有重跑 `prepare-package.sh`。重跑脚本，然后走
   「授权当前精确制品 → 确认并升级」。同版本号只改内容时，卡片上的「重启」不会切换制品。
3. 想确认手上这份二进制是谁写的——两条命令，三种签名（体积是 2026-11-18 的实测值，
   工具链/代码一变就跟着变，只看「哪种签名」不看绝对数）：

   ```bash
   go version -m plugins/sforum.qq-bot/backend/plugin | head -1        # 工具链
   go version -m plugins/sforum.qq-bot/backend/plugin | grep -F '=>'   # replace 形态
   ```

   出现 `=> ../../../../SForum/apps/api`（相对）＝ `prepare-package.sh` 的产物，首行 `go1.27.1`，约 25.5 MB；
   出现绝对路径 `=> /…/SForum/apps/api` ＝ 两个工作流（CI / release）的产物，首行 `go1.26.11`，约 24.5 MB；
   整行不出现 ＝ 宿主 dev 的产物，首行 `go1.26.11`，约 17.0 MB（`-ldflags=-s -w` 已剥符号）。

## 12. 被动回复与主动消息频控（官方「发送群聊消息」页口径）

M1.3 加「被 @ 回一句说明」时按官方文档核对的事实，写在这里备查（代码见
`backend/gateway.go` 的 `replyToMention` 与 `backend/qqclient.go` 的
`SendPassiveGroupMessage`）：

- **被动回复靠 `msg_id`**：取自 `GROUP_AT_MESSAGE_CREATE` 事件的 `d.id`，事件发生后
  **5 分钟内**有效，**不占主动消息频控**；同一条消息最多被回复 **5 次**。
- **`msg_seq` 与 `msg_id` 联合去重**：相同组合重复发送会失败（`40054005`）。我们固定
  `msg_seq=1`，靠网关侧「每群 60 秒最多一次」保证同一 `msg_id` 只回一次——固定值让平台
  成为第二道去重防线，改成自增反而会绕开它。
- **`event_id` 也能做被动回复**（与 `msg_id` 二选一），支持的事件包含 `GROUP_ADD_ROBOT`：
  「机器人被拉进群」时回欢迎语是可行的，本轮未做。
- **群侧接口清单**：按 `group_openid` 的只有 `info` / `bot_state` / `members` / `messages`
  / `files` / 撤回等，**没有「列出机器人所在群」**——所以「已识别群列表」只能用事件记录
  （`ClaimRecord`）+ 设置页勾选这条路。
- **主动消息频控**（本插件不改代码，只备查）：Bot 维度企业/个人认证 60 qpm、未认证 30 qpm；
  单关系 20 qpm；**每群每天最多接收 1000 条**。插件默认 20/分钟 + 每群每日 800，落在限内。
- 被动回复相关错误码翻译：`40034005` / `304103` → 被动回复窗口已过期；`40034128` →
  主动消息受限或被动回复时间/次数用尽；`40054005` → 该消息已被回复过；
  `40034101` / `40054003` → 机器人不在该群里。

来源：[发送群聊消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_messages.post.html)、
[获取群基本信息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/api/v2_groups_group_openid_info.get.html)。

## 13. 字段类型与平台实际返回不一致（0.4.0 线上离线事故）

**事故**：0.4.0 线上机器人一直离线，后台徽标显示「连接异常 / 取凭证接口返回了无法解析
的内容」，QQ 端「连接到QQ Bot」页卡在连接中。根因是一行结构体：

```go
ExpiresIn int `json:"expires_in"`   // 平台实际返回的是字符串
```

腾讯 `POST https://api.bot.qq.com/app/getAppAccessToken`（`bots.qq.com` 同）实测返回
`{"access_token":"…","expires_in":"7111"}`——**`expires_in` 是字符串**。`json.Unmarshal`
直接报 `cannot unmarshal string into Go struct field .expires_in of type int`，于是
`AccessToken()` 落到「返回了无法解析的内容」，网关拿不到 token、无法 identify，机器人
永远离线。

**修法**：`qqclient.go` 的 `flexInt`（数字 / 整数字符串 / 空串 / null 都能读，其余形状
报错且不回显字段值）；取凭证的错误文案拆成「不是 JSON（带 HTTP 状态、Content-Type、
字节数）」「字段形状不对」「没有返回 access_token」三种，body 原文与 token 一律不进错误。

**教训（比修法本身更值钱）**：所有单测夹具当时都用 `"expires_in": 7200`（数字），于是
**夹具的假形状掩盖了平台的真实形状**。以后凡是外部接口的字段，夹具至少覆盖两种：文档
写的那种、以及实测抓到的那种。没有实测样本时，宁可按「两种都可能」写解析（`flexInt`
就是这种形状），也不要按文档赌一次。

### 13.1 同类待办（当前无线上证据，未改）

| 位置 | 字段 | 判断 |
| --- | --- | --- |
| `backend/bind.go:236` | `Retcode int` | q.qq.com lite 两个端点（`/lite/create_bind_task`、`/lite/poll_bind_result`）**没有官方文档**，字段形状只来自本仓库调研附录（`docs/qq-api-v2-capabilities.md` §11.1 写的是 `"status": 0` 数字）。若平台改成 `"status":"2"`，症状与本次 token 事故一模一样：外层信封解不开或轮询落到「扫码服务返回了未知的绑定状态」。修法同 `flexInt`，但会牵动 bindstore / 路由层的状态判断与既有测试，**当前无线上证据，留待办**。 |
| `backend/bind.go:289` | `Status int` | 同上，轮询状态码。 |
| `backend/gateway.go:83-84` | `Op int` / `S int64` | 官方 gateway 文档明确是数字；且形状变了会先在连接层暴露（心跳/续接直接失败），不会像 token 那样静默卡死。暂不动。 |
| `backend/gateway.go:360` | `heartbeat_interval int64` | 同上，官方文档明确是数字。暂不动。 |

已核对无误（文档声明与代码一致，不列待办）：`SendResult.ID`（消息 id 字符串）、
`BotProfile{ID,Username,Avatar}`（全 string + `bot` bool）、群信息 `group_name`（string）、
`GatewayURL.url`（string）、各处 `err_code` / `code`（数字，且已做双字段兜底）。
