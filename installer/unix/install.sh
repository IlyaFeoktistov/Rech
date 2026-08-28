#!/bin/sh
# Устанавливает компилятор Rech (rech) на Linux/macOS: собирает из
# исходников этого репозитория (go build) и кладёт бинарник в каталог,
# который пользователь укажет через --prefix (по умолчанию ~/.local/bin -
# без sudo, обычно уже в PATH современных дистрибутивов/оболочек). Не
# устанавливает C-компилятор/Boehm GC сам - это внешние зависимости самой
# ОС (см. docs/MANUAL.md, "Зависимости для сборки"), только проверяет их
# наличие и подсказывает команду установки под конкретную ОС/пакетный
# менеджер, если чего-то не нашлось.
#
# POSIX sh (не bash) - запускается и через "sh install.sh", и через
# "./install.sh" на системах, где /bin/sh - dash (Debian/Ubuntu) или
# ash (Alpine).
set -eu

PREFIX="${HOME}/.local/bin"
for arg in "$@"; do
    case "$arg" in
        --prefix=*) PREFIX="${arg#--prefix=}" ;;
        --help|-h)
            echo "Использование: install.sh [--prefix=КАТАЛОГ]"
            echo "  --prefix=КАТАЛОГ   куда положить бинарник rech (по умолчанию \$HOME/.local/bin)"
            exit 0
            ;;
        *)
            echo "неизвестный аргумент: $arg" >&2
            exit 1
            ;;
    esac
done

REPO_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

os_name() {
    uname -s
}

fail() {
    echo "ошибка: $1" >&2
    exit 1
}

echo "Rech: установка из $REPO_ROOT в $PREFIX"

if ! command -v go >/dev/null 2>&1; then
    fail "не найден Go (нужен для сборки rech из исходников) - установите с https://go.dev/dl/ и повторите"
fi

# Проверка C-компилятора - тот же порядок поиска, что и у самого
# internal/toolchain.CC (env $CC, затем cc/clang/gcc в PATH): если ни
# одного нет, "rech build"/"rech run" не смогут собрать ни одну
# Rech-программу (компилятор транспилирует в C, финальная сборка - через
# системный компилятор), хотя сам install.sh это переживёт.
CC_BIN=""
if [ -n "${CC:-}" ] && command -v "$CC" >/dev/null 2>&1; then
    CC_BIN="$CC"
else
    for candidate in cc clang gcc; do
        if command -v "$candidate" >/dev/null 2>&1; then
            CC_BIN="$candidate"
            break
        fi
    done
fi

case "$(os_name)" in
    Linux)
        PKG_HINT="sudo apt install build-essential libgc-dev   # Debian/Ubuntu
       sudo dnf install gcc gc-devel              # Fedora
       sudo pacman -S base-devel gc                # Arch"
        ;;
    Darwin)
        PKG_HINT="xcode-select --install   # C-компилятор (clang)
       brew install bdw-gc      # Boehm GC"
        ;;
    *)
        PKG_HINT="(неизвестная ОС - см. docs/MANUAL.md, \"Зависимости для сборки\")"
        ;;
esac

if [ -z "$CC_BIN" ]; then
    echo "предупреждение: не найден C-компилятор (\$CC, cc, clang, gcc) - rech соберётся," >&2
    echo "  но собранные ИМ Rech-программы не смогут дособраться до бинарника. Установите:" >&2
    echo "       $PKG_HINT" >&2
else
    echo "C-компилятор найден: $CC_BIN"
fi

# Boehm GC (libgc) - нужна КАЖДОЙ скомпилированной Rech-программе (см.
# docs/MANUAL.md) - probe пробной компиляцией/линковкой крошечной
# программы с "#include <gc.h>" через уже найденный компилятор, а не
# поиском конкретных путей/имён пакетов (они разные на каждой ОС и
# дистрибутиве).
if [ -n "$CC_BIN" ]; then
    PROBE_DIR="$(mktemp -d)"
    trap 'rm -rf "$PROBE_DIR"' EXIT
    cat > "$PROBE_DIR/probe.c" <<'EOF'
#include <gc.h>
int main(void) { GC_INIT(); return 0; }
EOF
    if "$CC_BIN" "$PROBE_DIR/probe.c" -lgc -o "$PROBE_DIR/probe" >/dev/null 2>&1; then
        echo "Boehm GC (libgc) найдена"
    else
        echo "предупреждение: не найдена Boehm GC (libgc, нужна каждой собранной Rech-программе) - установите:" >&2
        echo "       $PKG_HINT" >&2
    fi
fi

mkdir -p "$PREFIX"
echo "Сборка rech..."
(cd "$REPO_ROOT" && go build -o "$PREFIX/rech" ./cmd/rech)
chmod +x "$PREFIX/rech"
echo "Установлено: $PREFIX/rech"

case ":$PATH:" in
    *":$PREFIX:"*)
        echo "$PREFIX уже в PATH - готово."
        ;;
    *)
        RC_FILE="$HOME/.profile"
        case "${SHELL:-}" in
            */zsh) RC_FILE="$HOME/.zshrc" ;;
            */bash) RC_FILE="$HOME/.bashrc" ;;
        esac
        EXPORT_LINE="export PATH=\"$PREFIX:\$PATH\""
        if [ -f "$RC_FILE" ] && grep -qF "$EXPORT_LINE" "$RC_FILE" 2>/dev/null; then
            echo "$PREFIX уже добавлен в PATH в $RC_FILE - откройте новый терминал."
        else
            printf '\n# добавлено установщиком Rech\n%s\n' "$EXPORT_LINE" >> "$RC_FILE"
            echo "$PREFIX добавлен в PATH через $RC_FILE - откройте новый терминал (или выполните: . $RC_FILE)."
        fi
        ;;
esac

echo "Проверка: $("$PREFIX/rech" version 2>&1 || echo "не удалось запустить rech version")"
