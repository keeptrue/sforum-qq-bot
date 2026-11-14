# sforum.qq-bot

SForum 扩展包：让论坛与 QQ 双向联动（站内内容推送到 QQ 群，QQ 侧发言按规则回流论坛）。

面向使用者的完整说明（安装、配置）见仓库根 `README.md`；设计取舍与踩坑记录见仓库根
`NOTES.md`，宿主约束与开发约定见 `AGENTS.md`。

## 包内结构

```text
sforum.extension.json      Manifest V3（待落地）
manifest/langs/            zh-CN / en-US 的扩展名与描述（待落地）
backend/                   Protocol V2 服务（独立 Go module，待落地）
brand/                     icon.svg / icon.png / social-preview.png（待设计）
```

> **当前状态：M0 仓库骨架。** 上述四项均未提交内容，因此当前没有可安装的扩展包。
> 目录先占位是为了让 CI、打包脚本与发布流水线的路径在业务代码落地前就固定下来。

## 快速验证

```bash
cd backend && go vet ./... && go test ./...
cd ../../.. && ./scripts/prepare-package.sh
```

## 约定

- 扩展 id 固定为 `sforum.qq-bot`，包目录名与清单 `id` 必须一致。
- 凭据（AppID / AppSecret）只进后台设置页，不写进清单或仓库。
- 改任何包内文件（含品牌图）都要重跑 `prepare-package.sh`：宿主按 exact-artifact 校验摘要。
