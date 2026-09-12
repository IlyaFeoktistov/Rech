#!/bin/sh
# Собирает настоящий .deb-пакет rech (dist/rech_<версия>_<arch>.deb) -
# в отличие от installer/unix/install.sh (который только собирает rech из
# исходников и ПРОВЕРЯЕТ наличие внешних библиотек), этот пакет через
# Depends заставляет apt/dpkg САМ поставить всё нужное для сборки любой
# Rech-программы (см. control.template) вместе с самим rech - один "apt
# install ./rech_*.deb", без ручных подсказок.
#
# POSIX sh - запускается и как "sh build.sh", и через "make deb"
# (Makefile в корне).
set -eu

REPO_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)"
DEB_DIR="$REPO_ROOT/installer/linux/deb"
DIST_DIR="$REPO_ROOT/dist"

fail() {
    echo "ошибка: $1" >&2
    exit 1
}

command -v dpkg-deb >/dev/null 2>&1 || fail "не найден dpkg-deb - нужен для сборки .deb (пакет dpkg на любом Debian/Ubuntu)"
command -v go >/dev/null 2>&1 || fail "не найден Go - нужен для сборки самого rech"

# Версия - "БАЗА+git<short sha>": БАЗА берётся из internal/version/
# version.go (Version = "X") - единственная точка правды, не задваивать
# число ещё и здесь; "+git<sha>" добавлен, потому что голый short SHA
# невалиден как Debian upstream-версия, если начинается с буквы (a-f), а
# "БАЗА+git<sha>" валиден всегда и соответствует ровно тому, что реально
# зашивается в бинарник тем же -ldflags (см. doc-комментарий version.go).
BASE_VERSION="$(sed -n 's/^var Version = "\(.*\)"$/\1/p' "$REPO_ROOT/internal/version/version.go")"
[ -n "$BASE_VERSION" ] || fail "не удалось прочитать internal/version/version.go"
GIT_SHA="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo unknown)"
VERSION="${VERSION:-${BASE_VERSION}+git${GIT_SHA}}"

# Архитектура - "dpkg --print-architecture" (архитектура ЭТОЙ машины) по
# умолчанию, переопределима через ARCH= - GOARCH называется так же, как
# и Debian-архитектура, для amd64/arm64 (единственных, которые здесь
# поддерживаются), отдельной таблицы соответствий не нужно.
ARCH="${ARCH:-$(dpkg --print-architecture)}"
case "$ARCH" in
    amd64|arm64) ;;
    *) fail "неподдерживаемая архитектура: $ARCH (поддерживаются amd64/arm64)" ;;
esac

PKGROOT="$(mktemp -d)"
trap 'rm -rf "$PKGROOT"' EXIT

echo "Rech .deb: версия $VERSION, архитектура $ARCH"

mkdir -p "$PKGROOT/usr/bin" "$PKGROOT/usr/share/rech/zoneinfo" "$PKGROOT/usr/share/doc/rech" "$PKGROOT/DEBIAN"

echo "Сборка rech (GOARCH=$ARCH)..."
(cd "$REPO_ROOT" && GOOS=linux GOARCH="$ARCH" go build \
    -ldflags "-X github.com/IlyaFeoktistov/Rech/internal/version.Version=$VERSION" \
    -o "$PKGROOT/usr/bin/rech" ./cmd/rech)

# Часовые пояса (IANA tzdata) - "$(dirname /usr/bin)/share/rech/zoneinfo" =
# "/usr/share/rech/zoneinfo", ровно тот путь, что вычисляет tzdataDir()
# (internal/backend/cbackend/tzdata.go) для любого бинарника Unix-Rech,
# установленного в "<префикс>/bin".
cp -r "$REPO_ROOT/runtime/zoneinfo/." "$PKGROOT/usr/share/rech/zoneinfo/"

cp "$REPO_ROOT/LICENSE" "$PKGROOT/usr/share/doc/rech/copyright"

sed -e "s/@VERSION@/$VERSION/" -e "s/@ARCH@/$ARCH/" \
    "$DEB_DIR/control.template" > "$PKGROOT/DEBIAN/control"

mkdir -p "$DIST_DIR"
OUT="$DIST_DIR/rech_${VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "$PKGROOT" "$OUT"
echo "Собран: $OUT"
