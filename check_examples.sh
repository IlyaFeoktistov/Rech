#!/usr/bin/env bash
# Смоук-тест "собираются ли все examples/*.rech" - тот же путь, что и у
# реального пользователя (rech build), не юнит-тесты компилятора (которые
# не гоняют настоящий cc, см. DEVELOPMENT.md, "Как тестировать"). Только
# компиляция (не запуск) - часть примеров интерактивна (читает stdin) или
# долго исполняется (обучение нейросети), это здесь не нужно.
#
# Пропускает: .rech-файлы внутри каталога проекта (.rechproj рядом или
# выше по дереву - "исходники/*.rech" не самостоятельные программы, их
# собирает только сам .rechproj целиком) и .rech-файлы без "функция
# главная" (библиотечные модули для "использовать ... из", не собираются
# отдельно - см. examples/импорты/утилиты.rech, импортируется из
# examples/импорты/главный.rech). Также пропускает проекты, чья
# "сборка.платформа" (docs/MANUAL.md, "Полный справочник полей .rechproj")
# не допускает хост этого прогона - "rech build" сам вернул бы ту же
# ошибку (internal/project.File.CheckHostPlatform), здесь это read-only
# проверка ДО вызова, чтобы вывести "ПРОПУЩЕНО", а не "ОШИБКА" - в отличие
# от условная_компиляция/ (файлы _linux/_windows выбираются САМИ, никогда
# не попадают в чужую сборку), examples/проекты/windows_окна безусловно
# зовёт WinAPI - собрать на Linux/macOS нечем (нет user32.dll/gdi32.dll),
# это не баг компилятора, а объявленное ограничение примера. Тот же
# принцип у missing_lib_header_projects ниже - examples/проекты/
# x11_клавиши зовёт libX11 (сборка.библиотеки_linux: X11), которого нет на
# машине без пакета "libx11-dev" (не гарантирован на голом CI-образе, в
# отличие от libssl-dev - нет смысла требовать его только ради одного
# демонстрационного примера).
set -uo pipefail

# platform_of - "сборка.платформа" из .rechproj $1, "текущая" если поле не
# указано (тот же дефолт, что и internal/project.File.Load).
platform_of() {
    local value
    value="$(awk -F: '/^[[:space:]]*платформа:/{gsub(/^[ \t]+|[ \t]+$/,"",$2); print $2; exit}' "$1")"
    echo "${value:-текущая}"
}

# is_platform_mismatched - true (0), если значение "сборка.платформа" $1
# не допускает хост этого прогона (go env GOOS) - та же таблица значений,
# что и internal/project.File (platformOSNames, package doc
# CheckHostPlatform): "текущая" - любой хост; "unix" - Linux ИЛИ macOS (не
# Windows); "linux"/"windows"/"macos" - ровно одна ОС. Неизвестное
# значение (опечатка и т. п.) сюда не попадает - return 1, пусть "rech
# build" сам выдаст настоящую ошибку валидации вместо тихого пропуска.
is_platform_mismatched() {
    local value="$1" goos
    goos="$(go env GOOS)"
    case "$value" in
    "текущая") return 1 ;;
    linux) [ "$goos" = "linux" ] && return 1 || return 0 ;;
    windows) [ "$goos" = "windows" ] && return 1 || return 0 ;;
    macos) [ "$goos" = "darwin" ] && return 1 || return 0 ;;
    unix)
        { [ "$goos" = "linux" ] || [ "$goos" = "darwin" ]; } && return 1
        return 0
        ;;
    *) return 1 ;;
    esac
}

missing_lib_header_projects=("examples/проекты/x11_клавиши:/usr/include/X11/Xlib.h")
is_missing_lib_header_skipped() {
    local pd="$1"
    local entry header
    for entry in "${missing_lib_header_projects[@]}"; do
        if [ "$pd" = "${entry%%:*}" ]; then
            header="${entry#*:}"
            [ -f "$header" ] && return 1
            return 0
        fi
    done
    return 1
}
cd "$(dirname "$0")"

tmp_bin="$(mktemp -d)/rech"
trap 'rm -rf "$(dirname "$tmp_bin")"' EXIT

echo "Сборка компилятора..."
if ! go build -o "$tmp_bin" ./cmd/rech; then
    echo "не удалось собрать компилятор" >&2
    exit 1
fi

total=0
failed=0
declare -a project_dirs=()

# lib_search_paths - каталоги "выход" уже собранных ".rechproj" с
# "сборка: тип: библиотека" (не закомментированных - т. е. строка
# начинается с "тип:", не "# тип:") - экспортируются как LIBRARY_PATH/
# LD_LIBRARY_PATH перед КАЖДОЙ следующей проверкой, чтобы проект-потребитель
# ("внешняя функция"/"сборка.библиотеки", docs/SPEC.md, разделы 12/12.1),
# идущий в сортировке ПОСЛЕ своей библиотеки (см. examples/проекты/
# экспорт_библиотеки_потребитель/), мог слинковаться против уже локально
# собранного .so/.dylib/.dll без системной установки - cc/ld сами читают
# оба этих переменных окружения (стандартное поведение, не Rech-специфика).
# Живой найденный баг - без "LC_ALL=C" (ниже, `find ... | LC_ALL=C sort`)
# порядок сортировки зависит от локали окружения (BSD sort на macOS-раннере
# CI сортирует кириллицу иначе, чем ожидает эта логика) - "закрытый_пакет_
# потребитель" мог попасть в списке РАНЬШЕ "закрытый_пакет" (библиотеки), и
# сборка потребителя падала "library ... not found", хотя порядок каталогов
# на диске был правильный. "LC_ALL=C" даёт чистую побайтовую сортировку
# везде одинаково - "/" (0x2F) всегда раньше "_" (0x5F), поэтому у файла
# библиотеки (.../закрытый_пакет/закрытый_пакет.rechproj) всегда меньший
# путь, чем у потребителя (.../закрытый_пакет_потребитель/...).
lib_search_paths=""

add_lib_search_path() {
    if [ -z "$lib_search_paths" ]; then
        lib_search_paths="$1"
    else
        lib_search_paths="$lib_search_paths:$1"
    fi
    export LIBRARY_PATH="$lib_search_paths"
    export LD_LIBRARY_PATH="$lib_search_paths"
}

check() {
    local path="$1"
    total=$((total + 1))
    local out
    if out=$("$tmp_bin" build "$path" </dev/null 2>&1 >/dev/null); then
        echo "OK      $path"
    else
        failed=$((failed + 1))
        echo "ОШИБКА  $path"
        echo "$out" | sed 's/^/        /'
    fi
}

echo
echo "== Проекты (.rechproj) =="
while IFS= read -r proj; do
    pd="$(dirname "$proj")"
    plat="$(platform_of "$proj")"
    if is_platform_mismatched "$plat"; then
        echo "ПРОПУЩЕНО $proj (сборка.платформа: $plat, хост $(go env GOOS) - не подходит)"
        project_dirs+=("$pd")
        continue
    fi
    if is_missing_lib_header_skipped "$pd"; then
        echo "ПРОПУЩЕНО $proj (нет заголовка сторонней библиотеки на этой машине)"
        project_dirs+=("$pd")
        continue
    fi
    check "$proj"
    project_dirs+=("$pd")
    if grep -qE '^[[:space:]]*тип:[[:space:]]*(статическая_)?библиотека' "$proj"; then
        outdir=$(awk -F: '/^[[:space:]]*выход:/{gsub(/^[ \t]+|[ \t]+$/,"",$2); print $2; exit}' "$proj")
        [ -n "$outdir" ] && add_lib_search_path "$pd/$outdir"
    fi
done < <(find examples -iname '*.rechproj' | LC_ALL=C sort)

is_in_project_dir() {
    local f="$1"
    local pd
    for pd in "${project_dirs[@]:-}"; do
        [[ "$f" == "$pd"/* ]] && return 0
    done
    return 1
}

echo
echo "== Отдельные файлы (.rech, .речь, .р) =="
while IFS= read -r f; do
    is_in_project_dir "$f" && continue
    grep -qF 'функция главная' "$f" || continue
    check "$f"
done < <(find examples \( -iname '*.rech' -o -iname '*.речь' -o -iname '*.р' \) | LC_ALL=C sort)

echo
echo "Итого: $total, ошибок: $failed"
[ "$failed" -eq 0 ]
