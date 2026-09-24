package project

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// platformSuffix - суффикс полей для текущей платформы ("_windows"/
// "_linux"/"_macos").
func platformSuffix() string {
	switch ast.CurrentPlatform() {
	case "Windows":
		return "_windows"
	case "MacOS":
		return "_macos"
	default:
		return "_linux"
	}
}

// КомпиляторC - "сборка.компилятор_c" (вариант для текущей платформы
// побеждает общий), "" - не указан.
func (pf *File) КомпиляторC() string {
	if v := pf.platformField(pf.cCompilerWindowsRaw, pf.cCompilerLinuxRaw, pf.cCompilerMacosRaw); v != "" {
		return v
	}
	return pf.cCompilerRaw
}

// ФлагиC - "сборка.флаги_c" плюс вариант текущей платформы. set - хотя бы
// одно из двух полей есть в файле: тогда флаги ПОЛНОСТЬЮ заменяют
// встроенные (см. compiler.CBuild).
func (pf *File) ФлагиC() (flags []string, set bool, err error) {
	return pf.flagsField("сборка.флаги_c", pf.cFlagsRaw, pf.platformField(pf.cFlagsWindowsRaw, pf.cFlagsLinuxRaw, pf.cFlagsMacosRaw))
}

// ФлагиЛинковки - "сборка.флаги_линковки" плюс вариант текущей платформы,
// тем же принципом, что ФлагиC.
func (pf *File) ФлагиЛинковки() (flags []string, set bool, err error) {
	return pf.flagsField("сборка.флаги_линковки", pf.linkFlagsRaw, pf.platformField(pf.linkFlagsWindowsRaw, pf.linkFlagsLinuxRaw, pf.linkFlagsMacosRaw))
}

func (pf *File) flagsField(name, common, platform string) ([]string, bool, error) {
	set := pf.present[name] || pf.present[name+platformSuffix()]
	if !set {
		return nil, false, nil
	}
	flags, err := splitFlags(common)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", name, err)
	}
	extra, err := splitFlags(platform)
	if err != nil {
		return nil, false, fmt.Errorf("%s%s: %w", name, platformSuffix(), err)
	}
	return append(flags, extra...), true, nil
}

// splitFlags делит строку флагов по пробелам; "..." - один флаг с
// пробелами внутри (путь), кавычки в сам флаг не входят.
func splitFlags(s string) ([]string, error) {
	var flags []string
	var cur strings.Builder
	inQuotes, has := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			has = true
		case (r == ' ' || r == '\t') && !inQuotes:
			if has {
				flags = append(flags, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if inQuotes {
		return nil, fmt.Errorf("незакрытая кавычка в %q", s)
	}
	if has {
		flags = append(flags, cur.String())
	}
	return flags, nil
}
