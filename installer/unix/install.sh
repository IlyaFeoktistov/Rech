#!/bin/sh
# Устанавливает компилятор Rech (rech) на Linux/macOS: собирает из
# исходников этого репозитория (go build) и кладёт бинарник в каталог,
# который пользователь укажет через --prefix (по умолчанию ~/.local/bin -
# без sudo, обычно уже в PATH современных дистрибутивов/оболочек). Не
# устанавливает C-компилятор/внешние библиотеки (Boehm GC, клиенты БД,
# OpenSSL, nghttp2, libpng, libjpeg) сам - это внешние зависимости самой ОС
# (см. README.md, "Зависимости для сборки"), только проверяет их наличие
# (probe - пробная компиляция/линковка крошечной программы через найденный
# компилятор) и подсказывает команду установки под конкретную ОС/пакетный
# менеджер, если чего-то не нашлось. Ни одно предупреждение не прерывает
# установку самого rech - все внешние библиотеки нужны не rech (Go-
# бинарнику), а ПРОГРАММАМ, которые он в будущем соберёт, и то не любой, а
# только использующей соответствующую фичу (см. таблицу в README.md).
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
        PKG_HINT="(неизвестная ОС - см. README.md, \"Зависимости для сборки\")"
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
# README.md) - probe пробной компиляцией/линковкой крошечной
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

# Клиентские библиотеки БД (libsqlite3/libpq/libmysqlclient) - нужны
# только программе, использующей "БД" (см. docs/SPEC.md, "БД") - тем же
# принципом probe-компиляции/линковки, что и Boehm GC выше, но без
# заголовка (cbackend не включает настоящие заголовки этих библиотек - см.
# internal/backend/cbackend/db.go, - достаточно проверить, что линковщик
# находит саму библиотеку по символу, реально в ней объявленному).
if [ -n "$CC_BIN" ]; then
    # PROBE_DIR - тот же временный каталог (и тот же EXIT-trap), что и у
    # пробы Boehm GC выше - trap с EXIT в POSIX sh не накапливается,
    # повторный "trap ... EXIT" заменил бы собой предыдущий и оставил бы
    # PROBE_DIR неудалённым.
    MISSING_DB_LIBS=""
    for spec in "sqlite3:sqlite3_open" "pq:PQconnectdb" "mysqlclient:mysql_init"; do
        lib="${spec%%:*}"
        symbol="${spec#*:}"
        cat > "$PROBE_DIR/probe.c" <<EOF
extern void *$symbol();
int main(void) { return 0; }
EOF
        if ! "$CC_BIN" "$PROBE_DIR/probe.c" "-l$lib" -o "$PROBE_DIR/probe" >/dev/null 2>&1; then
            MISSING_DB_LIBS="$MISSING_DB_LIBS -l$lib"
        fi
    done
    if [ -n "$MISSING_DB_LIBS" ]; then
        echo "предупреждение: не найдены клиентские библиотеки БД:$MISSING_DB_LIBS (нужны только программе," >&2
        echo "  использующей \"БД\" - подключиться_к_бд/запросить/выполнить/закрыть_бд) - установите:" >&2
        case "$(os_name)" in
            Linux)
                echo "       sudo apt install libsqlite3-dev libpq-dev libmysqlclient-dev   # Debian/Ubuntu" >&2
                echo "       sudo dnf install sqlite-devel libpq-devel mysql-devel           # Fedora" >&2
                ;;
            Darwin)
                echo "       brew install sqlite postgresql mysql-client" >&2
                ;;
            *)
                echo "       (неизвестная ОС - см. README.md, \"Зависимости для сборки\")" >&2
                ;;
        esac
    else
        echo "клиентские библиотеки БД (sqlite3/pq/mysqlclient) найдены"
    fi
fi

# Опциональные внешние библиотеки (OpenSSL/nghttp2/libpng/libjpeg) - нужны
# только программе, реально использующей соответствующую фичу (TLS/
# шифрование, HTTP2/gRPC, PNG/JPEG-кодек), но, в отличие от клиентских
# библиотек БД выше, cbackend ДЕЙСТВИТЕЛЬНО включает настоящие заголовки
# этих библиотек ("#include <openssl/ssl.h>" и т. п.) - значит и probe
# может честно include'ить тот же заголовок, не полагаясь на голое extern-
# объявление символа. probe_header_lib - общая функция для всех четырёх
# (тот же принцип "не копипастить", что и у остального install.sh) - первый
# аргумент - СПИСОК заголовков через пробел (не один - "jpeglib.h"
# ссылается на "FILE", объявленный в "stdio.h", сам его не подключает,
# ровно тот же порядок, что cbackend эмитирует в сгенерированном C, см.
# internal/backend/cbackend/ir_generator.go), остальные - флаги линковщика.
# Результат: 0 (найдена) - echo "$1 найдена/найден", 1 (не найдена) -
# ничего не печатает, оставляет вызывающему решить формулировку
# предупреждения.
probe_header_lib() {
    headers="$1"
    shift
    : > "$PROBE_DIR/probe.c"
    for h in $headers; do
        echo "#include <$h>" >> "$PROBE_DIR/probe.c"
    done
    echo "int main(void) { return 0; }" >> "$PROBE_DIR/probe.c"
    "$CC_BIN" "$PROBE_DIR/probe.c" "$@" -o "$PROBE_DIR/probe" >/dev/null 2>&1
}

if [ -n "$CC_BIN" ]; then
    if probe_header_lib "openssl/ssl.h" -lssl -lcrypto; then
        echo "OpenSSL (libssl/libcrypto) найден"
    else
        echo "предупреждение: не найден OpenSSL (нужен \"слушать_tcp_защищённо\"/TLS-клиенту/\"зашифровать\"/\"расшифровать\") - установите:" >&2
        case "$(os_name)" in
            Linux)
                echo "       sudo apt install libssl-dev      # Debian/Ubuntu" >&2
                echo "       sudo dnf install openssl-devel   # Fedora" >&2
                ;;
            Darwin) echo "       brew install openssl" >&2 ;;
            *) echo "       (неизвестная ОС - см. README.md, \"Зависимости для сборки\")" >&2 ;;
        esac
    fi

    if probe_header_lib "nghttp2/nghttp2.h" -lnghttp2; then
        echo "nghttp2 найден"
    else
        echo "предупреждение: не найден nghttp2 (нужен HTTP/2- и gRPC-функциям, \"http2_*\"/\"grpc_*\"/\"слушать_grpcs\") - установите:" >&2
        case "$(os_name)" in
            Linux)
                echo "       sudo apt install libnghttp2-dev      # Debian/Ubuntu" >&2
                echo "       sudo dnf install libnghttp2-devel    # Fedora" >&2
                ;;
            Darwin) echo "       brew install nghttp2" >&2 ;;
            *) echo "       (неизвестная ОС - см. README.md, \"Зависимости для сборки\")" >&2 ;;
        esac
    fi

    if probe_header_lib "png.h" -lpng; then
        echo "libpng найден"
    else
        echo "предупреждение: не найден libpng (нужен \"декодировать_изображение\"/\"закодировать_изображение\" с ФорматИзображения.PNG) - установите:" >&2
        case "$(os_name)" in
            Linux)
                echo "       sudo apt install libpng-dev      # Debian/Ubuntu" >&2
                echo "       sudo dnf install libpng-devel    # Fedora" >&2
                ;;
            Darwin) echo "       brew install libpng" >&2 ;;
            *) echo "       (неизвестная ОС - см. README.md, \"Зависимости для сборки\")" >&2 ;;
        esac
    fi

    if probe_header_lib "stdio.h jpeglib.h" -ljpeg; then
        echo "libjpeg найден"
    else
        echo "предупреждение: не найден libjpeg (нужен \"декодировать_изображение\"/\"закодировать_изображение\" с ФорматИзображения.JPEG) - установите:" >&2
        case "$(os_name)" in
            Linux)
                echo "       sudo apt install libjpeg-dev              # Debian/Ubuntu" >&2
                echo "       sudo dnf install libjpeg-turbo-devel      # Fedora" >&2
                ;;
            Darwin) echo "       brew install jpeg" >&2 ;;
            *) echo "       (неизвестная ОС - см. README.md, \"Зависимости для сборки\")" >&2 ;;
        esac
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
