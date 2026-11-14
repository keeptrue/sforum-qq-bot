# sforum.qq-bot

SForum 扩展包：让论坛与 QQ 双向联动（站内内容推送到 QQ 群，QQ 侧发言按规则回流论坛）。

面向使用者的完整说明（安装、配置）见仓库根 `README.md`；设计取舍与踩坑记录见仓库根
`NOTES.md`，宿主约束与开发约定见 `AGENTS.md`。

## 包内结构

```text
sforum.extension.json      Manifest V3（待落地）
manifest/langs/            zh-CN / en-US 的扩展名与描述（待落地）
backend/                   Protocol V2 服务（独立 Go module，待落地）
brand/                     icon.svg / icon.png / social-preview.png（已交付）
```

> **当前状态：M0 仓库骨架。** 品牌资产已交付并入库；仅 `sforum.extension.json`、
> `manifest/langs/` 与 `backend/` 三项尚未落地，因此当前没有可安装的扩展包。
> 目录先占位是为了让 CI、打包脚本与发布流水线的路径在业务代码落地前就固定下来。

## 快速验证

> **骨架期说明**：`backend/` 下还没有 `go.mod`，`prepare-package.sh` 会以 exit 1
> 明确提示缺什么。本节自 **M2（清单与后端落地那一步）起可用**，届时再照此执行。

```bash
cd backend && go vet ./... && go test ./...
cd ../../.. && ./scripts/prepare-package.sh
```

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
- 改任何包内文件（含品牌图）都要重跑 `prepare-package.sh`：宿主按 exact-artifact 校验摘要。
