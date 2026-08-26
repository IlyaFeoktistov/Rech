// Package semantic производит internal/ir.Program из ast.Program (шаг 5.b,
// docs/ROADMAP.md) - проверка типов и разрешение имён на отдельном слое,
// а не внутри backend'а, как сейчас (internal/backend/cbackend делает это
// сам во время генерации C, шаг 1).
//
// Реализуется вертикальным срезом за срезом по конструкциям языка - см.
// internal/ir, package doc. Analyze возвращает понятную ошибку "не
// реализовано в IR" для всего, что ещё не перенесено - это не тихая
// деградация, а явная граница текущего покрытия.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Analyze производит IR из AST. Покрытие на данный момент - шаги 0-4
// (`вывести`, переменные/арифметика, если/пока/для, функции с параметрами
// и типом результата, вызов, сущности/поля/конструктор/методы/"этот"),
// шаг 4.1 (состояния, упрощённая версия) и шаг 4.2 (встроенные generic-
// коллекции) - см. internal/ir, package doc. Расширяется вместе с
// internal/backend/cbackend.GenerateFromIR подшаг за подшагом, шаг 5.
func Analyze(prog *ast.Program) (*ir.Program, error) {
	a := &Analyzer{
		resolvedConsts:  map[string]ir.Expr{},
		resolvingConsts: map[string]bool{},
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
