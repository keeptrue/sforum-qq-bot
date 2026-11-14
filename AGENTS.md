# SForum QQ Bot

SForum 的 QQ 机器人插件：让论坛与 QQ 双向联动（消息接收、事件回调、指令应答）。
**外部扩展包**，不在 SForum 主仓库内，也不属于内置插件。

- 仓库：`~/Code/github/sforum-qq-bot`（独立仓库，仓库根即 SForum 的外部扩展源根）
- 框架检出：`~/Code/github/SForum`（**同级目录**，插件通过本地 `replace` 解析宿主 SDK）
- 扩展 id：`sforum.qq-bot`；版本在 `plugins/sforum.qq-bot/sforum.extension.json`
- 面向运营者的说明见仓库根 `README.md`，设计取舍与踩坑记录见 `NOTES.md`

> 当前状态：**M0 仓库骨架**。目录、CI、发布流水线已就位，插件后端（`backend/`）尚未落地，
> 因此清单与品牌资产还未提交。骨架阶段的 CI 会检测后端是否就位，未就位时只做结构检查并
> 发出提示，不判失败。

## 目录布局

```text
sforum-qq-bot/
├── plugins/sforum.qq-bot/         ← 扩展包：manifest、langs、backend、brand
│   ├── manifest/langs/           ← zh-CN / en-US 的扩展名与描述
│   ├── backend/                  ← Protocol V2 服务（独立 Go module，待建）
│   ├── brand/                    ← icon.svg / icon.png / social-preview.png（待设计）
│   └── README.md                 ← 包级说明
├── scripts/prepare-package.sh    ← build → digest --write → validate → brand check → test
├── .github/workflows/            ← ci.yml / release.yml
├── README.md  NOTES.md  AGENTS.md  CHANGELOG.md  LICENSE
└── .gitignore                    ← 忽略 backend/plugin 与 dist/
```

宿主扫描规则是 `<EXTERNAL_EXTENSION_ROOTS>/plugins/<包目录>`，**同一扩展 id 只能出现在
一个源**。宿主 `DigestTree` 禁止符号链接，不要用软链挂载，也不要保留第二份副本。

## 技术栈

- 后端：Go（独立 module，`protocolVersion: 2` / `hostApiVersion: sforum.host@2`），
  通过 hashicorp/go-plugin 与宿主通信。
- 清单：Manifest V3（`sforum.extension.json`）。
- 依赖宿主 SDK：`github.com/keeptrue/sforum/apps/api`，本地 `replace` 指向同级 SForum 检出。

## 命令

```bash
# 后端单测（后端落地后）
cd plugins/sforum.qq-bot/backend && go vet ./... && go test ./...

# 构建二进制 + 刷新摘要 + 宿主校验（SFORUM_DIR=… 可覆盖框架路径）
./scripts/prepare-package.sh

# 让站点加载：重启 API 扫描到候选 → 后台「插件管理 → 启用」
```

**改任何包内文件都必须重跑 `prepare-package.sh`**：宿主以 exact-artifact 校验哈希，
摘要不匹配的包会被拒绝启用。同版本号只改内容时，卡片上的「重启」不会切换制品，必须走
「升级（授权当前精确制品 → 确认并升级）」。

## 硬边界

1. **不写业务代码以外的越界东西**：本仓库只放 QQ 机器人的插件实现，不改宿主仓库。
2. **不在仓库里提交构建产物**：`backend/plugin` 与 `dist/` 由 `.gitignore` 挡住，
   发布包由 CI 交叉编译产出。
3. **摘要不是装饰**：`backend/plugin` 的摘要写进清单，宿主按它做 exact-artifact 校验；
   摘要漂移会让包被拒绝启用。
4. **构建参数三处必须一致**（宿主 dev watcher、`scripts/prepare-package.sh`、CI）：
   `CGO_ENABLED=0 go build -trimpath -buildvcs=false`。任何一处不同，同一份源码的摘要就不同。
5. **`backend/go.mod` 的 `go` 指令不得低于宿主 `apps/api/go.mod`**：插件是 replace 进宿主
   模块构建的，低了只会得到一句 `updates to go.mod needed`。
6. **不上传镜像凭据**：QQ 机器人 AppID / Token 走宿主设置项（`SFORUM_SETTING_*` 环境变量
   注入子进程），不写进仓库、不写进清单。
7. **只推 `main`**：发布走 `v*` 标签触发的工作流，不长期维护其它分支。

## CI 与发布

- `ci.yml`：push `main` / PR / 手动触发。检出插件仓库 + 宿主框架（`SFORUM_REF`，默认 `main`）
  → `go mod edit -replace` 指向检出目录 → `go vet` / `go test` → 构建宿主平台二进制
  → 其余发布平台交叉编译冒烟 → `extension digest --write` / `validate` / `test`。
  摘要与提交值不一致只发 notice，不判失败（工具链差异常见）。
- `release.yml`：`v*` 标签（或手动）触发；`verify` → 四平台 `build` → `publish`
  （`SHA256SUMS` + Release）。标签版本必须等于清单 `version`，否则 verify 直接失败。
- 产物命名：`sforum.qq-bot_<version>_<os>_<arch>.zip`（附 SBOM）。
- 骨架阶段（后端与清单尚未落地）两个工作流都会跳过 Go 相关步骤并只发提示。

## 验证标准（不要只跑单测就报完成）

1. `cd plugins/sforum.qq-bot/backend && go test ./...`
2. `cd ~/Code/github/SForum/apps/api && go run ./cmd/sforum extension validate|test <包路径>`
3. 品牌资产门禁：`go run ./cmd/sforum extension brand check <包路径>`
4. 真实冒烟：在站内发一条触发 QQ 侧动作的内容，确认机器人按预期应答。
5. 品牌三张图由设计师产出，尺寸/体积上限见 `NOTES.md`。

## 设计取舍

取舍与踩坑的完整记录见 `NOTES.md`；这里只列当前已定的几条：

- **独立仓库而不是放进 SForum 主仓库**：插件有自己的发布节奏与信任边界；
  宿主只按「外部扩展源根」扫描，不为某个插件开特例。
- **QQ 侧凭据只进设置项**：清单里不出现任何 AppID / Secret，避免把可执行安装包
  变成凭据分发渠道。
- **CI 检出宿主源码而不是依赖 Go module**：宿主的 `apps/api` 尚未发布成 Go module，
  检出后用 `go mod edit -replace` 覆盖本地路径。
