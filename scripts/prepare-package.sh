#!/usr/bin/env bash
# 构建本机平台的插件二进制、刷新包摘要并跑一遍宿主契约校验。
# 用法：scripts/prepare-package.sh [extension-root]（默认 plugins/sforum.qq-bot）
# 环境变量：SFORUM_DIR=/path/to/SForum（宿主检出目录）、SKIP_FRONTEND=1（跳过前端构建）
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

FRONTEND_DIR="$PKG/frontend/admin"

# 设置页组件由 bun 构建，产物（dist）随源码提交入库：宿主不编译前端源码，只读 dist。
# 因此这里除了重建产物，还要核对它与提交内容一致——不一致说明产物没提交或不可复现。
if [ "${SKIP_FRONTEND:-0}" = "1" ]; then
  echo "==> 跳过前端构建（SKIP_FRONTEND=1）"
elif [ ! -d "$FRONTEND_DIR" ]; then
  echo "==> 跳过前端构建：$FRONTEND_DIR 不存在（骨架阶段没有前端产物）"
else
  if [ ! -f "$FRONTEND_DIR/bun.lock" ] && [ ! -f "$FRONTEND_DIR/bun.lockb" ]; then
    echo "找不到前端 lockfile：$FRONTEND_DIR/bun.lock" >&2
    echo "CI 用 bun install --frozen-lockfile 构建，lockfile 必须提交入库。" >&2
    exit 1
  fi
  echo "==> 构建 frontend/admin 产物（bun install --frozen-lockfile && bun run build）"
  (cd "$FRONTEND_DIR" && bun install --frozen-lockfile && bun run build)
  # 本地只告警：本机 bun 版本可能与 CI 不同，产物漂移先让人看见，不挡住打包。
  if [ -n "$(git -C "$FRONTEND_DIR" status --porcelain -- dist 2>/dev/null)" ]; then
    echo "警告：frontend/admin/dist 与提交内容不一致（产物未提交或构建不可复现）" >&2
    git -C "$FRONTEND_DIR" status --short -- dist >&2 || true
    echo "本地继续执行；CI 会把这种情况判为 ::error。" >&2
  fi
fi

# 依赖树必须在 digest/validate 之前物理删除：宿主算作者树摘要时用 DigestAuthorTree（跳过
# node_modules），而 exact-artifact 校验用 VerifiedTreeDigest（不跳过），同一棵树上两者必然
# 不一致，validate 会报 "package digest mismatch after static inspection"。
# .gitignore 只是不提交，磁盘上的树照样被算进去；这里删的是依赖树，dist/lockfile/源码不动。
if [ -d "$FRONTEND_DIR/node_modules" ]; then
  rm -rf "$FRONTEND_DIR/node_modules"
  echo "==> 已剔除 node_modules（宿主 digest 会把它算进制品树）"
fi

# 编译参数只有一组：CGO_ENABLED=0 + -trimpath + -buildvcs=false，不带 -ldflags；ci.yml / release.yml 必须同参。
# 摘要不保证跨环境相同——宿主 dev 构建（ExtensionPackage/dev_build.go:85）带 -ldflags=-s -w，
# 两个工作流还把 replace 重写成绝对路径。dev/CI 与本地制品摘要天然不同，属预期，不是 bug；
# 别为「对齐摘要」给这里加 -ldflags。证据、复现命令与判据见 NOTES.md §11。
echo "==> 构建 backend/plugin（CGO_ENABLED=0，与 ci.yml / release.yml 同参）"
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
