# 变更记录

`sforum.qq-bot` 的对外变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

发布说明由 `.github/workflows/release.yml` 从对应版本的小节生成，因此**打标签前必须先写这一节**，
否则 Release 只会有安装步骤。

## [Unreleased]

尚未发布的改动记在这里。

- 文档：回填官方 API 调研结论，收口功能任务书

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

- 后端实现、扩展清单 `sforum.extension.json` 与品牌三图（`icon.svg` / `icon.png` /
  `social-preview.png`）尚未提交，因此当前没有可安装的扩展包。
- 骨架阶段不承诺任何运行时行为；首个可用版本的功能清单会在实现落地后补写。
