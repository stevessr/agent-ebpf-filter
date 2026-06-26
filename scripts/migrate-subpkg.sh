#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# migrate-subpkg.sh — 将 app/ 平铺文件迁移到子包
#
# 用法:
#   ./scripts/migrate-subpkg.sh <pkgname> <file1.go> [file2.go ...]
#
# 示例:
#   ./scripts/migrate-subpkg.sh events \
#       contextutilsevent.go events_network.go event_flows.go
#
# 功能:
#   1. 创建 app/<pkgname>/ 目录
#   2. git mv 文件到子目录
#   3. 替换 package app → package <pkgname>
#   4. 添加 typebridge.go 类型别名（可选）
#   5. 用 gofmt 格式化
#   6. 增量构建检查
# ──────────────────────────────────────────────────────────────────────────────
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PKGNAME="$1"
shift

if [ -z "$PKGNAME" ]; then
    echo "Usage: $0 <pkgname> <file1.go> [file2.go ...]"
    exit 1
fi

SRCDIR="backend/app"
DSTDIR="backend/app/$PKGNAME"

# ── 1. 确保目标目录存在 ──────────────────────────────────────────────────────
mkdir -p "$DSTDIR"

# ── 2. 列出要迁移的文件 ──────────────────────────────────────────────────────
FILES=()
for f in "$@"; do
    # 处理带路径和不带路径的情况
    base="$(basename "$f")"
    if [ -f "$SRCDIR/$base" ]; then
        FILES+=("$base")
    else
        echo "⚠️  文件不存在: $SRCDIR/$base"
    fi
done

if [ ${#FILES[@]} -eq 0 ]; then
    echo "❌ 没有可迁移的文件"
    exit 1
fi

echo "═══════════════════════════════════════════════════════════════"
echo "  迁移到子包: $PKGNAME"
echo "  文件数:     ${#FILES[@]}"
for f in "${FILES[@]}"; do
    echo "    - $f"
done
echo "═══════════════════════════════════════════════════════════════"

# ── 3. git mv 每个文件 ──────────────────────────────────────────────────────
for f in "${FILES[@]}"; do
    # 同时迁移对应的 _test.go 文件
    base="${f%.go}"
    src="$SRCDIR/$f"
    dst="$DSTDIR/$f"

    # 主文件
    if [ -f "$src" ]; then
        git mv "$src" "$dst"
        echo "  ✔ git mv $f → $PKGNAME/"
    fi

    # 测试文件
    testfile="${base}_test.go"
    src_test="$SRCDIR/$testfile"
    if [ -f "$src_test" ]; then
        git mv "$src_test" "$DSTDIR/$testfile"
        echo "  ✔ git mv $testfile → $PKGNAME/ (测试文件)"
    fi
done

# ── 4. 替换 package 声明 ────────────────────────────────────────────────────
echo ""
echo "── 替换 package 声明 ──────────────────────────────────────────"
for f in "${FILES[@]}"; do
    dst="$DSTDIR/$f"
    if [ -f "$dst" ]; then
        sed -i "s/^package app$/package $PKGNAME/" "$dst"
        echo "  ✔ $f: package app → package $PKGNAME"
    fi

    # 测试文件
    base="${f%.go}"
    testfile="${base}_test.go"
    dst_test="$DSTDIR/$testfile"
    if [ -f "$dst_test" ]; then
        sed -i "s/^package app$/package $PKGNAME/" "$dst_test"
        echo "  ✔ $testfile: package app → package $PKGNAME"
    fi
done

# ── 5. gofmt 格式化 ─────────────────────────────────────────────────────────
echo ""
echo "── 格式化 ──────────────────────────────────────────────────────"
for f in "${FILES[@]}"; do
    dst="$DSTDIR/$f"
    [ -f "$dst" ] && gofmt -w "$dst" 2>/dev/null && echo "  ✔ gofmt $f" || true
    base="${f%.go}"
    testfile="${base}_test.go"
    dst_test="$DSTDIR/$testfile"
    [ -f "$dst_test" ] && gofmt -w "$dst_test" 2>/dev/null && echo "  ✔ gofmt $testfile" || true
done

# ── 6. 增量构建检查 ─────────────────────────────────────────────────────────
echo ""
echo "── 构建检查 ────────────────────────────────────────────────────"
cd "$ROOT/backend"
if go build ./app/... 2>&1; then
    echo "  ✅ 构建成功"
else
    echo ""
    echo "  ⚠️  构建失败 — 需要补充 typebridge.go 别名或修复依赖"
    echo "  常见原因:"
    echo "    - 导出的类型/函数需要在 typebridge.go 中添加类型别名"
    echo "    - 子包内部需要补充 import 语句"
    echo "    - 子包引用的全局变量需要通过 Deps 注入"
fi

# ── 7. 还原工作目录 ─────────────────────────────────────────────────────────
cd "$ROOT"
echo ""
echo "── 完成 ────────────────────────────────────────────────────────"
echo ""
echo "下一步:"
echo "  1. 检查构建失败的原因并修复"
echo "  2. 更新 app/routes.go 中的 import 路径（如需要）"
echo "  3. 更新 app/main.go 中的 Init() / Deps 注入（如需要）"
echo "  4. 运行: cd backend && go test ./app/$PKGNAME/..."
