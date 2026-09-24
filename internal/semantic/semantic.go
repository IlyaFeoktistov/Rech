// Package semantic производит internal/ir.Program из ast.Program - проверка
// типов и разрешение имён на отдельном слое, а не внутри backend'а.
//
// Analyze возвращает понятную ошибку "не реализовано в IR" для всего, что
// ещё не перенесено - это не тихая деградация, а явная граница текущего
// покрытия.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Analyze производит IR из AST.
func Analyze(prog *ast.Program) (*ir.Program, error) {
	// "#если ОС.<Член> { ... }" (docs/SPEC.md) - обычно уже разрешён
	// раньше, для КАЖДОГО отдельного файла, ДО их слияния в один prog
	// (internal/module, package doc ast.ResolvePlatformBlocks) -
	// идемпотентно (PlatformBlocks уже nil после первого разрешения) -
	// повторный вызов здесь ничего не меняет для программы, прошедшей
	// module.Load, но нужен вызывающим semantic.Analyze НАПРЯМУЮ, минуя
	// module (например, тестовые хелперы вроде internal/interp.runSrc).
	ast.ResolvePlatformBlocks(prog, ast.CurrentPlatform())

	// Синонимы типов - раньше generic'ов: цель синонима может быть
	// generic-инстанциацией ("синоним Ящик = Коробка<Целое>"), её обязана
	// увидеть instantiateGenerics (см. package doc synonyms.go).
	if err := expandSynonyms(prog); err != nil {
		return nil, err
	}

	// Вывод аргументов типа (generic_infer.go) - перезапуском анализа, пока
	// анализ находит новые выводы.
	numberCallNodes(prog)
	inf := &genericInference{table: map[string][]string{}}
	for range maxInferencePasses {
		inf.found = false
		irProg, err := analyzeOnce(prog, inf)
		if !inf.found {
			return irProg, err
		}
	}
	return nil, errNoPos(CodeGenerics, "вывод аргументов типа не сошёлся за %d проходов - укажите аргументы типа явно (\"имя<Тип>(...)\")", maxInferencePasses)
}

// analyzeOnce - один проход анализа (см. Analyze) с уже известными
// выводами аргументов типа inf.
func analyzeOnce(prog *ast.Program, inf *genericInference) (*ir.Program, error) {
	// Пользовательские generic-сущности/функции ("Коробка<T>") - раскрыты
	// в обычные объявления ДО всего остального: дальше по конвейеру ничего
	// не знает, что они когда-то были generic (см. package doc
	// internal/semantic/generics.go).
	instantiated, constraints, err := instantiateGenerics(prog, inf)
	if err != nil {
		return nil, err
	}
	prog = instantiated

	// Строчный - до collectInterfaces (см. package doc stringer.go): дальше
	// ведёт себя как обычный объявленный интерфейс программы. В копию, не
	// в prog вызывающего: повторный Analyze того же AST (LSP, частичный
	// анализ) иначе видит "Строчный" объявленным дважды.
	withStringer := *prog
	injectStringerInterface(&withStringer)
	prog = &withStringer

	a := &Analyzer{
		resolvedConsts:        map[string]ir.Expr{},
		resolvingConsts:       map[string]bool{},
		resolvedFieldDefaults: map[string]ir.Expr{},
		genericConstraints:    constraints,
		inference:             inf,
		callablesSeen:         map[string]bool{},
	}

	// Интерфейсы - до сущностей: поле/параметр сущности может
	// быть интерфейсного типа, значит interfaceSig должны быть уже готовы
	// (см. package doc internal/semantic/interfaces.go про обратное
	// ограничение - интерфейс не может ссылаться на сущность).
	if err := a.collectInterfaces(prog); err != nil {
		return nil, err
	}

	if err := a.collectEntities(prog); err != nil {
		return nil, err
	}

	if err := a.collectFuncSigs(prog); err != nil {
		return nil, err
	}

	if err := a.collectEnums(prog); err != nil {
		return nil, err
	}

	if err := a.collectModuleConsts(prog); err != nil {
		return nil, err
	}

	irProg, err := a.buildIR(prog)
	if err != nil {
		return nil, err
	}
	if err := validateExports(irProg); err != nil {
		return nil, err
	}
	return irProg, nil
}
