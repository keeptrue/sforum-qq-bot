# 变更记录

`sforum.qq-bot` 的对外变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

发布说明由 `.github/workflows/release.yml` 从对应版本的小节生成，因此**打标签前必须先写这一节**，
否则 Release 只会有安装步骤。

## [Unreleased]

尚未发布的改动记在这里。

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
