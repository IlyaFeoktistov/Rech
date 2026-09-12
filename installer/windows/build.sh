#!/bin/sh
# POSIX-аналог build.ps1 - кросс-собирает rech.exe и rech-setup.exe С
# Linux/macOS/WSL (go и makensis кросс-платформенны, PowerShell не нужен).
# build.ps1 остаётся для запуска НА настоящем Windows - этот скрипт не
# заменяет его, а добавляет путь для тех, кто режет релиз не с Windows.
set -eu

REPO_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

fail() {
    echo "ошибка: $1" >&2
    exit 1
}

command -v go >/dev/null 2>&1 || fail "не найден Go"
command -v makensis >/dev/null 2>&1 || fail "не найден makensis (NSIS) - sudo apt install nsis / winget install NSIS.NSIS"

BASE_VERSION="$(sed -n 's/^var Version = "\(.*\)"$/\1/p' "$REPO_ROOT/internal/version/version.go")"
[ -n "$BASE_VERSION" ] || fail "не удалось прочитать internal/version/version.go"
GIT_SHA="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo unknown)"
VERSION="${VERSION:-${BASE_VERSION}+git${GIT_SHA}}"

echo "Building rech.exe (версия $VERSION)..."
(cd "$REPO_ROOT" && GOOS=windows GOARCH=amd64 go build \
    -ldflags "-X github.com/IlyaFeoktistov/Rech/internal/version.Version=$VERSION" \
    -o installer/windows/rech.exe ./cmd/rech)

echo "Building rech-setup.exe..."
(cd "$REPO_ROOT/installer/windows" && makensis setup.nsi)

echo "Done: $REPO_ROOT/rech-setup.exe"
