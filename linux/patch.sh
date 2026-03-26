#!/bin/bash

# =============================================================================
# Max Font Patcher for Linux
# =============================================================================
# Usage:
#   ./patch.sh                          -- patch with defaults (size=13)
#   ./patch.sh --size 14                -- custom size
#   ./patch.sh --style list             -- list all available styles
#   ./patch.sh --path /opt/max          -- custom install path
# =============================================================================

STYLES="BodyStrong,MarkdownMessageMonospace"
SIZE=13
APP_DIR="/usr/share/max"

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --path|-p) APP_DIR="$2"; shift ;;
        --style|-s) STYLES="$2"; shift ;;
        --size|-z) SIZE="$2"; shift ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

APP_DIR=${APP_DIR%/}
TARGET="$APP_DIR/bin/max"
BAK="$APP_DIR/bin/max.bak"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCHER="$SCRIPT_DIR/font-patcher"

if [ "$STYLES" == "list" ]; then
    echo "📜 Reading available font styles from $TARGET..."
    "$PATCHER" -binary "$TARGET" -style list -no-backup
    exit 0
fi

echo "=== Max Font Patcher (Linux) ==="
echo "App:    $APP_DIR"
echo "Styles: $STYLES"
echo "Size:   $SIZE px"
echo "--------------------------------"

if [ ! -f "$TARGET" ]; then
    echo "❌ Error: File $TARGET not found. Is Max installed?"
    echo "   Install: sudo dnf install MAX"
    exit 1
fi

# 1. Create backup (only once, from the original clean binary)
if [ ! -f "$BAK" ]; then
    echo "📦 Creating first backup of original binary..."
    sudo cp "$TARGET" "$BAK"
fi

# 2. Restore clean binary from backup before patching
echo "🔄 Restoring clean binary from backup..."
sudo cp "$BAK" "$TARGET"

# 3. Apply the patch
LH=$((SIZE + 4))
echo "⚙️ Patching font sizes (px=$SIZE, lh=$LH) into JSON resource..."
sudo "$PATCHER" -binary "$TARGET" -style "$STYLES" -size "$SIZE" -line-height "$LH" -no-backup

if [ $? -ne 0 ]; then
    echo "❌ Patching failed."
    exit 1
fi

echo "✅ Done! You can now launch Max."
