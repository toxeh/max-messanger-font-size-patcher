# Max Font Patcher

Утилита для изменения размера шрифтов в мессенджере **Max** (VK Teams) на всех платформах.

Работает путём прямой модификации сжатого JSON-ресурса `typography.json` внутри бинарника приложения. Поддерживает 38 стилей типографики (чат, заголовки, моноширинный код и т.д.).

## Быстрый старт

### macOS

```bash
cd mac
./patch.sh                          # по умолчанию: size=13, стили BodyStrong + MarkdownMessageMonospace
./patch.sh --size 14                # другой размер
./patch.sh --style list             # список всех доступных стилей
./patch.sh --path ~/Applications/Max.app  # нестандартный путь установки
```

### Windows

```bat
cd win
patch.bat                           &:: по умолчанию: size=13
patch.bat --size 14                 &:: другой размер
patch.bat --style list              &:: список стилей
patch.bat --path "D:\Max"           &:: нестандартный путь
```

### Linux

```bash
cd linux
sudo ./patch.sh                     # по умолчанию: size=13
sudo ./patch.sh --size 14           # другой размер
sudo ./patch.sh --style list        # список стилей
sudo ./patch.sh --path /opt/max     # нестандартный путь
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
3. **Минимизирует** JSON и удаляет блок `ax5` (accessibility) для экономии места
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
cd mac && go build -o font-patcher-mac .

# Windows (кросс-компиляция)
cd win && GOOS=windows GOARCH=amd64 go build -o font-patcher.exe .

# Linux (кросс-компиляция)
cd linux && GOOS=linux GOARCH=amd64 go build -o font-patcher .
```

## Структура проекта

```
font-patcher/
├── README.md
├── mac/
│   ├── main.go              # Go-патчер (zlib + codesign)
│   ├── go.mod
│   ├── patch.sh             # Обёртка
│   └── font-patcher-mac     # Скомпилированный бинарник
├── win/
│   ├── main.go              # Go-патчер (zlib, без подписи)
│   ├── go.mod
│   ├── patch.bat            # Обёртка (.bat)
│   └── font-patcher.exe     # Скомпилированный бинарник
└── linux/
    ├── main.go              # Go-патчер (zstd, level 22)
    ├── go.mod / go.sum
    ├── patch.sh             # Обёртка
    └── font-patcher         # Скомпилированный бинарник
```
