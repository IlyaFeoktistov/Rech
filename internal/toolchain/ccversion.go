package toolchain

import (
	"fmt"
	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Фиксация поведения компилятора C. Сгенерированный C рассчитан на диалект
// GNU C11 (выражения-блоки "({ ... })", __int128, __attribute__, asm-метки)
// и не должен зависеть от того, какой стандарт и какие предупреждения
// компилятор включает по умолчанию: gcc 15 сменил стандарт по умолчанию на
// gnu23, gcc 14 сделал ошибками несовместимые указатели и неявные
// объявления. Поэтому:
//   - стандарт задаётся явно (-std=gnu11);
//   - то, что новые gcc считают ошибкой, - ошибка на любой версии
//     (-Werror=...): программа, собравшаяся на старом gcc, собирается и на
//     новом;
//   - версия компилятора проверяется: ниже минимальной - понятная ошибка;
//   - семантика языка не зависит от уровня оптимизации: переполнение целых
//     оборачивается по модулю (-fwrapv, docs/SPEC.md раздел 5 - в C
//     знаковое переполнение не определено, и -O2 выкидывает проверки,
//     рассчитывая на его отсутствие), приведения указателей между
//     структурами рантайма (сущности, Значение, FFI) не ломаются
//     оптимизатором (-fno-strict-aliasing).

// ccStrictFlags - встроенные флаги C для gcc/clang (заменяются целиком
// "сборка.флаги_c" проекта).
var ccStrictFlags = []string{
	"-std=gnu11",
	"-Werror=implicit-function-declaration",
	"-Werror=implicit-int",
	"-Werror=incompatible-pointer-types",
	"-Werror=int-conversion",
	"-fwrapv",
	"-fno-strict-aliasing",
}

// minCCVersion - наименьшие поддерживаемые версии (major); проверенные -
// TestedCCVersions.
var minCCVersion = map[string]int{"gcc": 9, "clang": 11}

// TestedCCVersions - на чём реально прогоняются тесты и примеры.
const TestedCCVersions = "gcc 13 (Linux), gcc 16 (Windows, MinGW-w64 UCRT)"

type ccInfo struct {
	family string // "gcc", "clang" или "" - другой компилятор (флаги не добавляются)
	major  int
}

var (
	ccInfoMu    sync.Mutex
	ccInfoCache = map[string]ccInfo{}
	versionRe   = regexp.MustCompile(`(\d+)\.\d+(\.\d+)?`)
)

// identifyCC - семейство и major-версия компилятора path (по "--version").
func identifyCC(path string) ccInfo {
	ccInfoMu.Lock()
	defer ccInfoMu.Unlock()
	if info, ok := ccInfoCache[path]; ok {
		return info
	}
	info := ccInfo{}
	// #nosec G204 -- путь к компилятору выбран локальным пользователем.
	out, err := exec.Command(path, "--version").Output()
	if err == nil {
		text := string(out)
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "clang"):
			info.family = "clang"
		case strings.Contains(lower, "gcc") || strings.Contains(lower, "free software foundation"):
			info.family = "gcc"
		}
		if m := versionRe.FindStringSubmatch(firstLine(text)); m != nil {
			info.major, _ = strconv.Atoi(m[1])
		}
	}
	ccInfoCache[path] = info
	return info
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// checkCCVersion - ошибка, если компилятор известного семейства старше
// минимальной поддерживаемой версии.
func checkCCVersion(path string, info ccInfo) error {
	min, known := minCCVersion[info.family]
	if !known || info.major == 0 || info.major >= min {
		return nil
	}
	return fmt.Errorf("компилятор C %s - %s %d, а нужен %s %d или новее (проверено: %s)", path, info.family, info.major, info.family, min, TestedCCVersions)
}

// cFlags - флаги C сборки: "сборка.флаги_c" проекта, если заданы, иначе
// встроенные - строгие и режима сборки (modeCFlags), только у gcc/clang:
// другой компилятор их может не знать.
func cFlags(b compiler.CBuild, info ccInfo) []string {
	if b.CFlagsSet {
		return append([]string{}, b.CFlags...)
	}
	if info.family != "" {
		return append(append([]string{}, ccStrictFlags...), modeCFlags(b)...)
	}
	return nil
}
