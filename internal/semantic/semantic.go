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
	// Пользовательские generic-сущности/функции ("Коробка<T>") - раскрыты
	// в обычные объявления ДО всего остального: дальше по конвейеру ничего
	// не знает, что они когда-то были generic (см. package doc
	// internal/semantic/generics.go).
	instantiated, constraints, err := instantiateGenerics(prog)
	if err != nil {
		return nil, err
	}
	prog = instantiated

	a := &Analyzer{
		resolvedConsts:     map[string]ir.Expr{},
		resolvingConsts:    map[string]bool{},
		genericConstraints: constraints,
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

	return a.buildIR(prog)
}
