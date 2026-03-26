#!/bin/bash

# Задаем дефолтные значения
STYLES="BodyStrong,MarkdownMessageMonospace"
SIZE=13
APP_DIR="/Applications/Max.app"

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --path|-p) APP_DIR="$2"; shift ;;
        --style|-s) STYLES="$2"; shift ;;
        --size|-z) SIZE="$2"; shift ;;
        *) echo "Неизвестный параметр: $1"; exit 1 ;;
    esac
    shift
done

# Убираем возможный слеш в конце
APP_DIR=${APP_DIR%/}
APP_PATH="$APP_DIR/Contents/MacOS"
TARGET="$APP_PATH/Max"
BAK="$APP_PATH/Max.bak"

if [ "$STYLES" == "list" ]; then
    echo "📜 Чтение списка доступных шрифтов из $TARGET..."
    ./font-patcher-mac -binary "$TARGET" -style list
    exit 0
fi

echo "=== Max.app Font Patcher ==="
echo "App:    $APP_DIR"
echo "Styles: $STYLES"
echo "Size:   $SIZE px"
echo "----------------------------"

if [ ! -f "$TARGET" ]; then
    echo "❌ Ошибка: Файл $TARGET не найден. Max.app установлен?"
    exit 1
fi

# 1. Если бекапа еще нет, создаем его (оригинальный чистый бинарник)
if [ ! -f "$BAK" ]; then
    echo "📦 Создание первого бекапа оригинального файла..."
    sudo cp "$TARGET" "$BAK"
fi

# 2. Восстанавливаем чистый файл из бекапа перед патчем.
# Это решает проблему "patched JSON is too large" при повторных запусках.
echo "🔄 Восстановление чистого бинарника из бекапа..."
sudo cp "$BAK" "$TARGET"

# 3. Применяем патч
LH=$((SIZE + 4))
echo "⚙️ Вшивание новых размеров (px=$SIZE, lh=$LH) в JSON ресурс..."
sudo ./font-patcher-mac -binary "$TARGET" -style "$STYLES" -size "$SIZE" -line-height "$LH"

if [ $? -ne 0 ]; then
    echo "❌ Ошибка во время работы патчера."
    exit 1
fi

# 4. Сброс карантина и переподпись
echo "🔓 Обновление цифровых подписей (macOS Gatekeeper)..."
sudo xattr -cr "$APP_DIR" 2>/dev/null || true

# Подписываем ТУПО ТОЛЬКО 1 ИЗМЕНЕННЫЙ БИНАРНИК (без --deep), чтобы не ломать QtFrameworks
sudo codesign --force --sign - "$TARGET"

if [ $? -eq 0 ]; then
    echo "✅ Готово! Можешь открывать Max.app."
else
    echo "⚠️ Внимание: Подпись завершилась с ошибкой, приложение может не открыться."
fi
