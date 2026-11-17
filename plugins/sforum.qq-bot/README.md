# sforum.qq-bot

SForum 扩展包（Manifest V3 + Protocol V2）：把论坛新主题推送到 QQ 群，支持手机 QQ
扫码绑定机器人凭据，设置页由预构建组件提供（含模板实时预览）。

面向运营者的安装、配置与排错说明见仓库根 `README.md`；设计取舍与查证记录见 `NOTES.md`；
仓库约定与开发流程见 `AGENTS.md`。

## 包内结构

```text
sforum.extension.json      Manifest V3：events / routes / 权限 / aiTools / packageFiles
manifest/settings.json     设置页 schema（ui.mode=component，字段同时作为表单回退）
manifest/langs/            zh-CN / en-US 的扩展名与描述
schemas/                   契约真源：10 个路由 schema + 2 个事件 schema
openapi/routes.yaml        路由片段：把 …@1 契约引用绑到 schemas/*.json
backend/                   Protocol V2 服务（独立 Go module，Go 侧零第三方依赖）
frontend/admin/            设置页组件（Vite + TypeScript，src 与 dist 都入库）
brand/                     icon.svg / icon.png / social-preview.png
```

## 它做什么

- 观察宿主 `topic.created`，按模板渲染后推送到群白名单；摘要走 HostQuery，不读宿主数据库。
- 手机 QQ 扫码一键绑定：插件出码 → 轮询 → AES-256-GCM 解密，AppID / AppSecret 自动写回设置。
- 群 ID 自动识别：长连接记下 `group_openid`，设置页「推送状态」里可直接复制。
- 设置页预构建组件：扫码卡片、模板变量点选、实时预览、底部预览 / 测试 / 状态。
- 组件未授权时宿主回退声明式表单，功能不丢（少二维码与实时预览）。

本期未实现：QQ → 论坛的入站回流、频道与单聊分支、投递记录导出 CSV。

## 本地构建与验证

```bash
# 后端单测（在仓库根执行）
cd plugins/sforum.qq-bot/backend && go vet ./... && go test ./...

# 设置页组件（在仓库根执行）：dist 必须入库，宿主不编译前端源码
cd plugins/sforum.qq-bot/frontend/admin && bun install --frozen-lockfile && bun run build

# 打包全链路（在仓库根执行）：前端构建 → 后端构建 → digest --write → validate → brand check → test
./scripts/prepare-package.sh
```

改包内任何文件（含 `frontend/admin/dist`、schema、openapi 片段、品牌图）都要重跑
`prepare-package.sh`：摘要写进清单，宿主按 exact-artifact 校验。脚本会在算摘要前物理删除
`frontend/admin/node_modules`——宿主算作者树摘要时跳过它、exact-artifact 校验不跳过，
留着必然报 `package digest mismatch after static inspection`。

## 品牌资产

三张图由本仓库自行绘制，不走宿主 `scripts/brand/generate.mjs` 模板：模板把主色写死为
SForum 主蓝 `#2671ed`，QQ 机器人需要自己的 QQ 蓝。矢量源是 `brand/icon.svg` 与
`docs/brand/social-preview.svg`。

- **配色**：底板渐变 `#1AC3F5 → #0B8CD6`（覆盖 QQ 官方蓝 `#12B7F5` 区间）；图形纯白
  `#FFFFFF`；机器人五官用深一档的 `#0B8CD6`。与 SForum 主品牌 `#2671ED`（偏靛）相比
  落在青蓝一侧，色相与明度都拉开，远看不会混。
- **构图**：主体是白色对话气泡 + 机器人面孔（天线、双眼、微笑），代表 QQ 官方机器人；
  右上角 45% 透明的圆角卡片（两道列表线）代表论坛主题帖。两块前后叠压，表达
  “论坛 ↔ 机器人”的双向流转。16px 下只剩气泡轮廓与两点眼睛，仍可辨。
- **版式**：社交卡沿用兄弟插件的排版密度 —— 眉标 `SFORUM · 扩展插件`、主标题、
  项目名 `sforum-qq-bot`、一句话定位、包名版本行，左栏在 `x=792` 处用竖线分栏，
  右侧 292×328 深色方块放主图形，底栏左侧 SForum 标识、右侧仓库 slug。

重新导出（依赖本机有 `google-chrome` 与 Pillow）：

```bash
cd plugins/sforum.qq-bot
google-chrome --headless=new --disable-gpu --no-sandbox --hide-scrollbars \
  --force-device-scale-factor=1 --default-background-color=00000000 \
  --window-size=512,512 --screenshot=brand/icon.png "file://$PWD/brand/icon.svg"
google-chrome --headless=new --disable-gpu --no-sandbox --hide-scrollbars \
  --force-device-scale-factor=1 --default-background-color=00000000 \
  --window-size=1280,640 --screenshot=brand/social-preview.png \
  "file://$PWD/docs/brand/social-preview.svg"
python3 -c "from PIL import Image;[Image.open(p).convert('RGBA').save(p,format='PNG',optimize=True) for p in ['brand/icon.png','brand/social-preview.png']]"
```

宿主门禁：`cd ~/Code/github/SForum/apps/api && go run ./cmd/sforum extension brand check <包路径>`。

## 约定

- 扩展 id 固定为 `sforum.qq-bot`，包目录名与清单 `id` 必须一致。
- 凭据（AppID / AppSecret）只进后台设置页，不写进清单或仓库。
- 路由的 `…@1` 契约引用必须同时落在 `schemas/*.json` 与 `openapi/routes.yaml` 的
  `components.schemas`，且每条路由都要在片段里有 operation；缺一个 `extension validate`
  就报 `route schema catalog: … route schema is missing`。
- 发布前核对清单 `version` 与 tag 一致。
