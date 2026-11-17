# SForum QQ Bot

SForum 的 QQ 机器人插件：论坛新主题推送到 QQ 群，并把机器人能看到的群 ID 记下来
供运营复制，支持手机 QQ 扫码一键绑定。
**外部扩展包**，不在 SForum 主仓库内，也不属于内置插件。

- 仓库：`~/Code/github/sforum-qq-bot`（独立仓库，仓库根即 SForum 的外部扩展源根）
- 框架检出：`~/Code/github/SForum`（**同级目录**，插件通过本地 `replace` 解析宿主 SDK）
- 扩展 id：`sforum.qq-bot`；版本在 `plugins/sforum.qq-bot/sforum.extension.json`
- 面向运营者的说明见仓库根 `README.md`，设计取舍与踩坑记录见 `NOTES.md`

> 当前状态：**`0.2.0`（M1 新帖推送 + M1.1 扫码绑定 / 组件设置页）**。
> 清单、后端（Protocol V2）、设置页组件与契约（schemas + openapi 片段）全部就位并通过
> 宿主 `extension validate` / `test` / `brand check`；入站回流、频道/单聊、CSV 导出未做
> （见 `docs/plan-qq-bot.md` §13 的交付状态）。

## 目录布局

```text
sforum-qq-bot/
├── docs/                          ← 任务书、调研结论与品牌矢量源
│   ├── plan-qq-bot.md             ← 产品任务书（§13 回填交付状态）
│   ├── qq-api-v2-capabilities.md  ← QQ 官方机器人 API v2 能力盘点（含扫码绑定 lite 附录）
│   └── brand/social-preview.svg   ← 社交卡矢量源
├── plugins/sforum.qq-bot/         ← 扩展包
│   ├── sforum.extension.json      ← Manifest V3：events / routes / 权限 / aiTools / packageFiles
│   ├── manifest/settings.json     ← 设置页 schema（ui.mode=component + 表单回退）
│   ├── manifest/langs/            ← zh-CN / en-US 的扩展名与描述
│   ├── schemas/                   ← 事件与路由的 JSON Schema（契约真源）
│   ├── openapi/routes.yaml        ← 路由片段：把 `…@1` 契约引用绑到 schemas/*.json
│   ├── backend/                   ← Protocol V2 服务（独立 Go module）
│   ├── frontend/admin/            ← 设置页组件（Vite + TypeScript；src 与 dist 都入库）
│   ├── brand/                     ← icon.svg / icon.png / social-preview.png
│   └── README.md                  ← 包级说明与品牌导出命令
├── scripts/prepare-package.sh     ← 前端构建 → 后端构建 → digest --write → validate → brand → test
├── .github/workflows/             ← ci.yml / release.yml
├── README.md  NOTES.md  AGENTS.md  CHANGELOG.md  LICENSE
└── .gitignore                     ← 忽略 backend/plugin 与 node_modules；**不**忽略 dist
```

宿主扫描规则是 `<EXTERNAL_EXTENSION_ROOTS>/plugins/<包目录>`，**同一扩展 id 只能出现在
一个源**。宿主 `DigestTree` 禁止符号链接，不要用软链挂载，也不要保留第二份副本。

## 技术栈

- 后端：Go（独立 module，`protocolVersion: 2` / `hostApiVersion: sforum.host@2`），
  通过 hashicorp/go-plugin 与宿主通信；**Go 侧零第三方依赖**（扫码解密的 AES-256-GCM
  只用标准库，新增依赖仅允许 `github.com/gorilla/websocket`，当前也未引入）。
- 前端：Vite + TypeScript，构建产物 `frontend/admin/dist/settings.{mjs,css}` 随源码入库；
  宿主不编译前端源码。二维码用 `uqr` 画 SVG，不引 Vue。
- 清单：Manifest V3（`sforum.extension.json`）+ 设置文档 `manifest/settings.json`。
- 依赖宿主 SDK：`github.com/keeptrue/sforum/apps/api`，本地 `replace` 指向同级 SForum 检出。

## 命令

```bash
# 后端单测
cd plugins/sforum.qq-bot/backend && go vet ./... && go test ./...

# 设置页组件：dist 必须入库，重建后要能复现同一份产物
cd plugins/sforum.qq-bot/frontend/admin && bun install --frozen-lockfile && bun run build

# 回到仓库根，确认 dist 与提交内容一致（有输出=产物没提交或构建不可复现）
git status --short plugins/sforum.qq-bot/frontend/admin/dist

# 打包全链路（前端 + 后端 + 摘要 + 宿主三项校验；SFORUM_DIR=… 可覆盖框架路径）
./scripts/prepare-package.sh
```

宿主门禁单独跑（`../SForum` 是同级框架检出）：

```bash
cd ../SForum/apps/api
go run ./cmd/sforum extension validate ../../../sforum-qq-bot/plugins/sforum.qq-bot
go run ./cmd/sforum extension test     ../../../sforum-qq-bot/plugins/sforum.qq-bot
go run ./cmd/sforum extension brand check ../../../sforum-qq-bot/plugins/sforum.qq-bot
```

## 硬边界

1. **不写业务代码以外的越界东西**：本仓库只放 QQ 机器人的插件实现，不改宿主仓库。
2. **只让一种产物入库**：`backend/plugin` 不入库（`.gitignore` 挡住），
   `frontend/admin/dist` **必须入库**——宿主只读 dist、不编译前端源码；
   `node_modules` 既不入库、也必须在算摘要前**物理删除**（`DigestAuthorTree` 跳过它、
   `VerifiedTreeDigest` 不跳过，留着必然报 `package digest mismatch after static inspection`）。
3. **摘要不是装饰**：`backend.digest` 与每个 `packageFiles[].digest` 都参与 exact-artifact 校验；
   改了包内任何文件（含前端 dist、schema、openapi 片段、品牌图）都要重跑 `prepare-package.sh`。
   同版本号只改内容时，卡片上的「重启」不会切换制品，必须走「升级（授权当前精确制品 → 确认并升级）」。
4. **构建参数三处必须一致**（宿主 dev watcher、`scripts/prepare-package.sh`、CI）：
   `CGO_ENABLED=0 go build -trimpath -buildvcs=false`。任何一处不同，同一份源码的摘要就不同。
5. **`backend/go.mod` 的 `go` 指令不得低于宿主 `apps/api/go.mod`**：插件是 replace 进宿主
   模块构建的，低了只会得到一句 `updates to go.mod needed`。
6. **契约引用与 openapi 片段必须成对**：路由的 `requestSchema` / `responseSchema` 用
   `…@1` 形式时，条目必须同时写进 `openapi/routes.yaml` 的 `components.schemas`，
   且**每条路由都要在片段里有 operation**；否则 `extension validate` 报
   `route schema catalog: … route schema is missing`。片段只做绑定，schema 内容只在 `schemas/*.json`。
7. **凭据只进设置项**：AppID / AppSecret 走宿主设置项（`SFORUM_SETTING_*` 注入子进程），
   不写进仓库、清单、日志、投递记录或状态文本；AppSecret 只在完成绑定的那次 `/bind/poll`
   响应里出现一次。
8. **只推 `main`**：发布走 `v*` 标签触发的工作流，不长期维护其它分支。

## CI 与发布

- `ci.yml`：push `main` / PR / 手动触发。`skeleton` 查根级文件与包目录；`verify` 检出插件仓库 +
  宿主框架（`SFORUM_REF`，默认 `main`）→ 前端 `bun install --frozen-lockfile && bun run build`
  并**比对提交的 dist**（漂移或未提交即失败）→ 删 `node_modules` → `go mod edit -replace` 指向检出目录
  → `go vet` / `go test` → 构建宿主平台二进制 → 其余发布平台交叉编译冒烟
  → `extension digest --write` / `validate` / `test`。摘要与提交值不一致只发 notice，不判失败
  （工具链差异常见）；前端产物不一致是硬失败。
- `release.yml`：`v*` 标签（或手动）触发；`verify` → 四平台 `build` → `publish`
  （`SHA256SUMS` + Release + SBOM）。要求后端、清单、前端三件齐备，标签版本必须等于清单
  `version`，打包前重建前端并校验 dist、删 `node_modules`，再用 `extension package --exclude-source`
  产出 zip。
- 产物命名：`sforum.qq-bot_<version>_<os>_<arch>.zip`。

## 验证标准（不要只跑单测就报完成）

1. `cd plugins/sforum.qq-bot/backend && go test ./...`
2. `cd plugins/sforum.qq-bot/frontend/admin && bun run build`，且 `git status --short …dist` 为空
3. `./scripts/prepare-package.sh`（构建 + 摘要 + validate + brand check + test 一次跑完）
4. 真实冒烟：站内发一篇新主题，白名单群收到推送；设置页「推送状态」能看到这条成功投递
5. 扫码冒烟：设置页「扫码绑定」出码 → 手机 QQ 扫 → 自动回填并保存 → 「发一条测试消息」返回成功

## 设计取舍

取舍与踩坑的完整记录见 `NOTES.md`（M1.1 的查证过程在 §10）；这里只列当前已定的几条：

- **独立仓库而不是放进 SForum 主仓库**：插件有自己的发布节奏与信任边界；
  宿主只按「外部扩展源根」扫描，不为某个插件开特例。
- **二维码必须走预构建组件**：探测结果只渲染成一行文本、探测 runtime 的 TMPDIR 是
  per-process 私有目录，无法承载「出码 → 轮询 → 回填」这条有状态的流程。
- **凭据只进设置项**：清单里不出现任何 AppID / Secret，避免把可执行安装包变成凭据分发渠道。
- **CI 检出宿主源码而不是依赖 Go module**：宿主的 `apps/api` 尚未发布成 Go module，
  检出后用 `go mod edit -replace` 覆盖本地路径。
