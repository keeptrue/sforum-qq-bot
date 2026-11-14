# SForum QQ Bot (`sforum.qq-bot`)

[![ci](https://github.com/keeptrue/sforum-qq-bot/actions/workflows/ci.yml/badge.svg)](https://github.com/keeptrue/sforum-qq-bot/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/keeptrue/sforum-qq-bot)](https://github.com/keeptrue/sforum-qq-bot/releases)

![SForum QQ Bot 社交卡](./plugins/sforum.qq-bot/brand/social-preview.png)

让 SForum 论坛与 QQ 双向联动：站内内容可推送到 QQ 群，QQ 侧的发言与指令可回流到论坛。

本仓库是**独立插件仓库**，仓库根即 SForum 的**外部扩展源根**，扩展包位于
`plugins/sforum.qq-bot/`。

> **当前状态：M0 仓库骨架。** CI 与发布流水线、品牌资产、插件后端均已规划，
> 品牌资产已交付（`icon.svg` / `icon.png` / `social-preview.png` 均已入库），
> 仅扩展清单 `sforum.extension.json` 与插件后端尚未落地。
> 现阶段安装包不可用；下面的「安装」「配置」「开发闭环」三节在首个可用版本发布后生效。

## 它解决什么问题

论坛用户分散在 QQ 群里：讨论、答疑、报名、发版通知都在那边发生。把 QQ 接进论坛的
收益是双向的：

- **出站**：论坛里的新主题、回复、公告可以按规则推到指定 QQ 群，不必人工搬运。
- **入站**：QQ 群里的发言可以按规则变成论坛回复，@ 机器人能直接执行论坛能力
  （查主题、查用户、触发动作）。

它**不**做「把 QQ 当成论坛的第二套权限系统」：论坛身份仍是论坛身份，QQ 侧的身份映射
由运营者显式配置，不做隐式认领。

## 安装

本插件是**外部扩展包**，不在 SForum 仓库内。两种装法二选一：
从 [Releases](https://github.com/keeptrue/sforum-qq-bot/releases) 下载与服务器平台
匹配的 zip，在后台「插件管理 → 安装插件」上传；或把本仓库直接挂成外部扩展源
（改代码时用这种）。

```bash
# 1. 构建二进制 + 刷新摘要 + 宿主校验（需要 SForum 检出在同级目录）
./scripts/prepare-package.sh

# 2. 让 SForum 扫描到本仓库（仓库根就是外部扩展源根）
#    在 SForum 的 .env 里追加（.env 已被 gitignore）
#    EXTERNAL_EXTENSION_ROOTS=...,/abs/path/to/sforum-qq-bot

# 3. 重启 API（外部源只在启动时扫描），然后
#    后台 → 扩展管理 → 插件管理 → 启用（需 super_admin 确认精确制品）
```

启用带后端的包需要 super_admin 的一次性 **exact-artifact 信任确认**：宿主按清单里
`backend.digest` 与包内可执行文件的实际摘要做比对，对不上就拒绝启用。

## 配置

所有配置走后台插件设置页，凭据以 `secret` 类型保存，并通过 `SFORUM_SETTING_*`
环境变量注入插件子进程（不写进仓库、不写进清单）。

> **骨架期说明**：扩展清单尚未落地，下表是设计草案，**后台设置页还不存在这些字段**。
> 本节自 **M2（清单与后端落地那一步）起可用**；按功能任务书（`docs/plan-qq-bot.md` §6）
> 的分期，清单与设置 schema、后端推送闭环是落地前的必经步骤，两步都完成后本表才成立。
> 届时以实际清单 schema 为准做一次字段对照。

| 设置 | 说明 |
| --- | --- |
| 启用机器人 | 总开关；关闭后插件保持启用但不产生任何外发行为 |
| AppID / AppSecret | QQ 开放平台应用凭据（`secret` 类型，掩码显示） |
| 机器人模式 | 群聊 / 私聊 / 两者；决定哪些会话允许入站回流 |
| 出站目标群 | 允许接收论坛推送的 QQ 群号列表 |
| 入站关键词前缀 | 只有以此前缀开头的 QQ 消息才回流论坛，避免群里闲聊污染论坛 |
| 单条消息最大长度 | 超过则截断，防止超长文本打爆渲染或触发审核队列 |
| 速率限制 | 每分钟出站消息条数上限 |

插件子进程的环境是**白名单**：`HTTP_PROXY` / `HTTPS_PROXY` 不会透传，
出站代理只能来自插件自己的设置项。

## 目录说明

```text
sforum-qq-bot/
├── docs/                          ← 任务书与调研结论
│   ├── plan-qq-bot.md             ← 产品任务书（473 行）
│   ├── qq-api-v2-capabilities.md  ← QQ 官方机器人 API v2 能力盘点（1069 行）
│   └── brand/social-preview.svg   ← 社交卡矢量源
├── plugins/sforum.qq-bot/         ← 扩展包
│   ├── manifest/langs/           ← zh-CN / en-US 的扩展名与描述
│   ├── backend/                  ← Protocol V2 服务（独立 Go module）
│   ├── brand/                    ← icon.svg / icon.png / social-preview.png
│   └── README.md                 ← 包级说明
├── scripts/prepare-package.sh    ← 本机构建 + 摘要刷新 + 宿主校验
├── .github/workflows/            ← ci.yml（验证）/ release.yml（发版）
├── AGENTS.md                     ← 仓库级 AI 约定
├── NOTES.md                      ← 决策与踩坑记录
├── CHANGELOG.md                  ← 变更记录
└── LICENSE                       ← MIT
```

## 开发闭环

> **骨架期说明**：本节命令分两段。**今天可跑**的只有下面第一段；第二段要等扩展清单与
> 后端落地之后才有意义，即 **M2（清单与后端落地那一步）起可用**。

今天可跑（骨架自检）：

```bash
bash -n scripts/prepare-package.sh   # 脚本语法检查
./scripts/prepare-package.sh         # 当前会 exit 1，并打印缺 manifest / backend 的具体原因
```

后端落地后（清单与 `backend/go.mod` 就位，M2 起）：

```bash
cd plugins/sforum.qq-bot/backend
go vet ./... && go test ./...

# 改任何包内文件后都必须重跑，宿主以 exact-artifact 校验哈希
cd ../../.. && ./scripts/prepare-package.sh
```

摘要一变，旧的信任授权即失效：后台会显示「升级」，需要重新走一次
「授权当前精确制品 → 确认并升级」。

## CI 与发布

[`ci.yml`](.github/workflows/ci.yml) 在每次推送到 `main` 和每个 PR 上跑：`go vet` / `go test`、
宿主平台的构建、发布矩阵其余平台的交叉编译冒烟，再用宿主 CLI 跑
`extension digest --write` / `validate` / `test`。构建出的摘要与仓库里提交的不一致时只发一条
notice（工具链差异常见），不判失败。

[`release.yml`](.github/workflows/release.yml) 由 `v*` 标签触发：

1. `verify`：vet/test + 宿主契约校验，并校验标签与 `sforum.extension.json` 的 `version` 一致。
2. `build`：矩阵交叉编译 linux/amd64、linux/arm64、darwin/arm64、darwin/amd64；每个平台各自
   写入清单摘要后打成 `sforum.qq-bot_<version>_<os>_<arch>.zip`（附 SBOM）。
3. `publish`：汇总产物、生成 `SHA256SUMS`、创建 Release 并上传。

发布前把 `sforum.extension.json` 的 `version` 提到与标签一致（例如 `0.1.0` ↔ `v0.1.0`）。
Windows 未列入矩阵：宿主按清单 `backend/entry` 直接执行、不做 `.exe` 归一化，
未经实机验证前不发布。

骨架期两个工作流的行为**不一样**：`ci.yml` 检测到后端或清单缺失时跳过对应步骤、
只发一条提示，不判失败；`release.yml` 的 `verify` 在后端或清单缺失时**直接失败**
（`::error` + exit 1，tag 推送与手动触发都跑）——发版本来就要求有可打包的东西，
这里不刻意放宽。

两个工作流都通过 `SFORUM_REF` 检出宿主框架（默认 `main`；Release 的 `workflow_dispatch`
可以覆盖）。宿主的 `apps/api` 没有发布成 Go module，所以 CI 检出框架后用
`go mod edit -replace` 把插件的 SDK 依赖指过去。

## 许可

[MIT](./LICENSE)
