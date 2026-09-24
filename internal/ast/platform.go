package ast

import "runtime"

// PlatformNames - три платформы, которые понимает компилятор (`internal/
// toolchain`) - порядок ЗНАЧИМ: это же порядок значений встроенного
// перечисления "ОС" (internal/semantic/types.go, StandardEnums["ОС"]) -
// единый источник для парсера (валидирует "ОС.<Член>" в "#если") и
// семантики (сам список значений перечисления), чтобы они не разъехались.
var PlatformNames = [...]string{"Linux", "Windows", "MacOS"}

// PlatformBlockDecl соответствует условной компиляции по целевой
// платформе - "#если ОС.<Член> { <объявления> } [#иначе если ОС.<Член>
// { ... }]... [#иначе { ... }]" (docs/SPEC.md) - декларация верхнего
// уровня, не инструкция: разрешается ДО семантического анализа (см.
// internal/module, ResolvePlatformBlocks) - объявления руки, не подошедшей
// под целевую платформу, НИКОГДА не доходят до остального конвейера
// (semantic/cbackend/interp) - как #ifdef-исключённый код в C, а не как
// обычный мёртвый код (который всё ещё проверяется компилятором).
//
// В отличие от рантайм "если"/"иначе" (docs/SPEC.md, "Цепочки `иначе
// если` нет" - вложенность через "если" внутри "иначе"), здесь ЦЕПОЧКА
// "#иначе если" разрешена: платформ ровно три (Linux/Windows/MacOS,
// перечисление "ОС"), трёхстороннее разбиение без цепочки было бы
// неудобной вложенностью, а сама эта директива - решается на этапе
// компиляции, а не во время исполнения, поэтому не связана изначальным
// решением про рантайм-`если`.
type PlatformBlockDecl struct {
	Arms []PlatformArm
	Line int
}

func (*PlatformBlockDecl) node() {}

// PlatformArm - одна "рука" PlatformBlockDecl - Platform="" только у
// финального безусловного "#иначе" (после всех "#если"/"#иначе если").
// Поля - те же верхнеуровневые списки объявлений, что несёт Program -
// ResolvePlatformBlocks копирует их туда для ПОДОШЕДШЕЙ руки как есть.
type PlatformArm struct {
	// Platform - "Linux"/"Windows"/"MacOS" (имя члена встроенного
	// перечисления "ОС", internal/semantic/types.go) - "" у безусловного
	// "#иначе".
	Platform       string
	Functions      []*FunctionDecl
	Entities       []*EntityDecl
	Uses           []*UseDecl
	ExternFuncs    []*ExternFuncDecl
	ExternEntities []*ExternEntityDecl
	Enums          []*EnumDecl
	Interfaces     []*InterfaceDecl
	Consts         []*ConstDecl
	Synonyms       []*SynonymDecl
}

// ResolvePlatformBlocks сплющивает prog.PlatformBlocks в обычные
// верхнеуровневые списки prog (Functions/Entities/Uses/ExternFuncs/
// ExternEntities/Enums/Interfaces/Consts) - для каждого блока ищет ПЕРВУЮ
// руку с Platform == current ИЛИ Platform == "" (безусловный "#иначе") -
// остальные руки того же блока отбрасывает целиком: их объявления НИКОГДА
// не попадают дальше (не в declaredNames, не в семантику, не в IR) - как
// #ifdef-исключённый код в C, а не как обычный, всё ещё проверяемый
// "мёртвый код". Вызывается ДО declaredNames (internal/module, package
// doc PlatformBlockDecl) - current - одно из PlatformNames.
func ResolvePlatformBlocks(prog *Program, current string) {
	for _, block := range prog.PlatformBlocks {
		for _, arm := range block.Arms {
			if arm.Platform != current && arm.Platform != "" {
				continue
			}
			prog.Functions = append(prog.Functions, arm.Functions...)
			prog.Entities = append(prog.Entities, arm.Entities...)
			prog.Uses = append(prog.Uses, arm.Uses...)
			prog.ExternFuncs = append(prog.ExternFuncs, arm.ExternFuncs...)
			prog.ExternEntities = append(prog.ExternEntities, arm.ExternEntities...)
			prog.Enums = append(prog.Enums, arm.Enums...)
			prog.Interfaces = append(prog.Interfaces, arm.Interfaces...)
			prog.Consts = append(prog.Consts, arm.Consts...)
			prog.Synonyms = append(prog.Synonyms, arm.Synonyms...)
			break
		}
	}
	prog.PlatformBlocks = nil
}

// PlatformIndex - индекс name (одно из PlatformNames) - то же число, что
// несёт соответствующий член встроенного перечисления "ОС"
// (internal/semantic/types.go, platformEnumMembers) - -1, если name не
// входит в PlatformNames.
func PlatformIndex(name string) int64 {
	for i, n := range PlatformNames {
		if n == name {
			return int64(i)
		}
	}
	return -1
}

// CurrentPlatform - GOOS хоста компилятора, отображённый в один из
// PlatformNames ("windows"->"Windows", "darwin"->"MacOS", остальное
// (linux и другие POSIX) -> "Linux") - общий источник для interp
// ("текущая_ос()", internal/interp/builtins.go) и загрузчика модулей
// ("#если ОС.<Член>"/"_windows.rech"-суффиксы, internal/module). Сегодня
// реальной кросс-компиляции нет ("сборка.платформа" `.rechproj` принимает
// только "текущая", internal/project/project.go) - значит "целевая
// платформа сборки" и "хост, на котором запущен сам компилятор" - буквально
// одно и то же везде, без расхождения cbackend/interp.
func CurrentPlatform() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "MacOS"
	default:
		return "Linux"
	}
}
