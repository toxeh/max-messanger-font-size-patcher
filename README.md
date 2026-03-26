# Max Font Patcher

Утилита для изменения размера шрифтов в мессенджере **Max** (VK Teams) на всех платформах.

Работает путём прямой модификации сжатого JSON-ресурса `typography.json` внутри бинарника приложения. Поддерживает 38 стилей типографики (чат, заголовки, моноширинный код и т.д.).

## Установка

Скачайте архив для вашей платформы со страницы [Releases](https://github.com/toxeh/max-messanger-font-size-patcher/releases/latest):

| Платформа | Архив |
|---|---|
| macOS (Apple Silicon) | `max-font-patcher-darwin-arm64.tar.gz` |
| macOS (Intel) | `max-font-patcher-darwin-amd64.tar.gz` |
| Linux (x86_64) | `max-font-patcher-linux-amd64.tar.gz` |
| Windows (x64) | `max-font-patcher-windows-amd64.zip` |

Распакуйте архив — внутри бинарник `font-patcher-*` и скрипт-обёртка (`patch.sh` / `patch.bat`).

## Быстрый старт

### macOS

```bash
tar xzf max-font-patcher-darwin-*.tar.gz
chmod +x font-patcher-darwin-* patch.sh
./patch.sh                                    # по умолчанию: size=13
./patch.sh --size 14                          # другой размер
./patch.sh --style list                       # список всех доступных стилей
./patch.sh --path ~/Applications/Max.app      # нестандартный путь установки
```

### Windows

```bat
:: Распаковать max-font-patcher-windows-amd64.zip
patch.bat                           &:: по умолчанию: size=13
patch.bat --size 14                 &:: другой размер
patch.bat --style list              &:: список стилей
patch.bat --path "D:\Max"           &:: нестандартный путь
```

### Linux

```bash
tar xzf max-font-patcher-linux-amd64.tar.gz
chmod +x font-patcher-linux-amd64 patch.sh
sudo ./patch.sh                               # по умолчанию: size=13
sudo ./patch.sh --size 14                     # другой размер
sudo ./patch.sh --style list                  # список стилей
sudo ./patch.sh --path /opt/max               # нестандартный путь
```

> **Требование:** на Linux должен быть установлен `zstd` (`sudo dnf install zstd`).

---

## Параметры

| Параметр | Описание | По умолчанию |
|---|---|---|
| `--size`, `-z` | Размер шрифта в пикселях | `13` |
| `--style`, `-s` | Стиль(и) через запятую, или `list` для вывода всех | `BodyStrong,MarkdownMessageMonospace` |
| `--path`, `-p` | Путь к приложению | Авто-детект |

## Как это работает

Приложение Max построено на Qt6. Все параметры шрифтов (размер, межстрочный интервал, вес) хранятся в JSON-ресурсе `typography.json`, который вкомпилирован в основной бинарник и сжат.

Патчер:

1. **Сканирует** бинарник в поисках сжатых потоков, содержащих `typography.json`
2. **Декомпрессирует** найденный ресурс
3. **Минимизирует** JSON для экономии места
4. **Подставляет** новые значения `font_size` / `line_height` через regex
5. **Сжимает обратно** и перезаписывает in-place (с проверкой, что новый блок ≤ оригинала)
6. **Обновляет** заголовок Qt-ресурса с новым размером

### Отличия между платформами

| | macOS | Windows | Linux |
|---|---|---|---|
| Бинарник | `Max.app/Contents/MacOS/Max` | `max.exe` | `/usr/share/max/bin/max` |
| Формат | Mach-O | PE | ELF |
| Сжатие ресурсов | **zlib** | **zlib** | **zstd** |
| Qt RCC заголовок | 8 байт (compLen+4, uncompLen) | 8 байт (compLen+4, uncompLen) | 4 байта (compLen) |
| Подпись | `codesign --force --sign -` | Не требуется | Не требуется |
| Права | `sudo` (SIP) | Админ (если в Program Files) | `sudo` (`/usr/share/`) |

### Почему Windows не требует подписи?

На macOS ядро **убивает** процесс с невалидной подписью — нужен `codesign`. На Windows сломанная Authenticode-подпись лишь убирает метку «Verified Publisher» — приложение запускается без проблем.

### Почему Linux использует zstd?

Qt6 на Linux собирается с поддержкой RCC v3, который использует zstd вместо zlib. Сжатие zstd на уровне 22 (ultra) значительно эффективнее — оригинальный JSON (45KB) сжимается до ~1.2KB. Go-библиотека `klauspost/compress` поддерживает максимум level 11, поэтому для Linux патчер вызывает системный `zstd -22 --ultra`.

## Сборка из исходников

```bash
# macOS (нативная сборка)
cd mac && go build -ldflags="-s -w" -o font-patcher-mac .

# Windows (кросс-компиляция)
cd win && GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o font-patcher.exe .

# Linux (кросс-компиляция)
cd linux && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o font-patcher .
```
