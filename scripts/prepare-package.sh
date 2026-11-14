#!/usr/bin/env bash
# 构建本机平台的插件二进制、刷新包摘要并跑一遍宿主契约校验。
# 用法：scripts/prepare-package.sh [extension-root]（默认 plugins/sforum.qq-bot）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG="${1:-$ROOT/plugins/sforum.qq-bot}"
SFORUM_DIR="${SFORUM_DIR:-$(cd "$ROOT/.." && pwd)/SForum}"

if [ ! -d "$SFORUM_DIR/apps/api" ]; then
  echo "找不到 SForum 检出目录：$SFORUM_DIR" >&2
  echo "可用 SFORUM_DIR=/path/to/SForum 指定，或把本仓库与 SForum 放在同一父目录下。" >&2
  exit 1
fi

if [ ! -f "$PKG/sforum.extension.json" ]; then
  echo "找不到扩展清单：$PKG/sforum.extension.json" >&2
  echo "骨架阶段（后端与清单尚未落地）无需打包；清单落地后重跑本脚本。" >&2
  exit 1
fi

if [ ! -f "$PKG/backend/go.mod" ]; then
  echo "找不到后端 module：$PKG/backend/go.mod" >&2
  echo "骨架阶段（后端尚未落地）无需打包；后端落地后重跑本脚本。" >&2
  exit 1
fi

# 与宿主 dev watcher（ExtensionPackage.BuildBackend）和 CI 同一组参数：
# CGO_ENABLED=0 + -buildvcs=false，否则同一份源码在两边产出不同字节、摘要永不收敛。
echo "==> 构建 backend/plugin（CGO_ENABLED=0，与宿主 dev 构建同参）"
(cd "$PKG/backend" && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$PKG/backend/plugin" ./)

echo "==> 刷新包摘要（extension digest --write）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension digest --write "$PKG")

echo "==> 校验清单（extension validate）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension validate "$PKG")

echo "==> 品牌资产检查（extension brand check）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension brand check "$PKG")

echo "==> 宿主契约检查（extension test）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension test "$PKG")

echo "完成：$PKG"
