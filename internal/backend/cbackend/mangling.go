package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// checkManglingCollisions ловит коллизию двух РАЗНЫХ Rech-имён,
// транслитерирующихся в один и тот же C-идентификатор (например, "ё"/"е"
// сворачиваются в translit одинаково) - явной ошибкой Rech, вызывается
// первым делом в GenerateFromIR, до генерации самого C. Без этой проверки
// такая коллизия доходит до настоящего cc невидимой ни для internal/semantic
// (там оперируют исходными именами, они различны), ни для generic C-кода -
// и там встречает читателя как "redefinition of 'v_elka'" без всякой связи
// с исходником на Rech.
//
// Порядок и структура проверки - тот же, что был у AST-пути (generator.go,
// удалён вместе с dead code cbackend.Generate): отдельный реестр на каждое
// C-пространство имён (функции, типы/сущности, поля и методы - каждой
// сущности свои, локальные переменные - в пределах одной области видимости
// функции/метода, с тем же затенением, что и в C).
func checkManglingCollisions(prog *ir.Program) error {
	funcMangled := map[string]string{}
	for _, fn := range prog.Functions {
		// Экспортированная функция - настоящее имя как есть (funcCName,
		// mangle.go), не mangleName - коллизия транслитерации её не
		// касается: раз она использует СВОЁ, единственное в своём роде
		// имя буквально, столкнуться она может только с ДРУГОЙ функцией с
		// БУКВАЛЬНО ТЕМ ЖЕ именем, что уже отдельно запрещено
		// семантическим анализом (два верхнеуровневых объявления с одним
		// именем).
		mangled := mangleName(fn.Name)
		if fn.Exported {
			mangled = fn.Name
		}
		if other, exists := funcMangled[mangled]; exists {
			return fmt.Errorf("функции %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одну из них", other, fn.Name, mangled)
		}
		funcMangled[mangled] = fn.Name

		if err := checkParamsAndBodyMangling(fn.Params, fn.Body); err != nil {
			return fmt.Errorf("функция %q: %w", fn.Name, err)
		}
	}

	entityMangled := map[string]string{}
	for _, ent := range prog.Entities {
		mangledType := mangleType(ent.Name)
		if ent.Exported {
			mangledType = ent.Name
		}
		if other, exists := entityMangled[mangledType]; exists {
			return fmt.Errorf("сущности %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одну из них", other, ent.Name, mangledType)
		}
		entityMangled[mangledType] = ent.Name

		fieldMangled := map[string]string{}
		for _, f := range ent.Fields {
			mangledField := mangleField(f.Name)
			if other, exists := fieldMangled[mangledField]; exists {
				return fmt.Errorf("сущность %q: поля %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одно из них", ent.Name, other, f.Name, mangledField)
			}
			fieldMangled[mangledField] = f.Name
		}

		methodMangled := map[string]string{}
		for _, m := range ent.Methods {
			if m.Exported {
				// Экспортированный метод - настоящее имя (exportedMethodCSymbol,
				// mangle.go), занимает ОБЩИЙ C-неймспейс с экспортированными
				// функциями/typedef'ами сущностей (funcMangled/entityMangled
				// выше), не приватную таблицу methodMangled этой сущности -
				// тем же принципом, что и у экспортированной функции выше.
				symbol := exportedMethodCSymbol(ent.Name, m.Name)
				if other, exists := funcMangled[symbol]; exists {
					return fmt.Errorf("метод %q сущности %q и объявление %q дают одно и то же C-имя %q - переименуйте одно из них", m.Name, ent.Name, other, symbol)
				}
				funcMangled[symbol] = fmt.Sprintf("%s.%s", ent.Name, m.Name)
				if err := checkParamsAndBodyMangling(m.Params, m.Body); err != nil {
					return fmt.Errorf("сущность %q: метод %q: %w", ent.Name, m.Name, err)
				}
				continue
			}
			mangledMethod := mangleMethod(ent.Name, m.Name)
			if other, exists := methodMangled[mangledMethod]; exists {
				return fmt.Errorf("сущность %q: методы %q и %q транслитерируются в одно и то же имя %q в C - переименуйте один из них", ent.Name, other, m.Name, mangledMethod)
			}
			methodMangled[mangledMethod] = m.Name

			if err := checkParamsAndBodyMangling(m.Params, m.Body); err != nil {
				return fmt.Errorf("сущность %q: метод %q: %w", ent.Name, m.Name, err)
			}
		}

		if ent.Ctor != nil {
			if ent.Ctor.Exported {
				// Экспортированный конструктор - тем же принципом, что и
				// экспортированный метод выше: настоящее имя
				// (exportedCtorCSymbol, mangle.go) в ОБЩЕМ C-неймспейсе с
				// экспортированными функциями/методами/typedef'ами, не
				// приватная таблица этой сущности.
				symbol := exportedCtorCSymbol(ent.Name)
				if other, exists := funcMangled[symbol]; exists {
					return fmt.Errorf("конструктор сущности %q и объявление %q дают одно и то же C-имя %q - переименуйте одно из них", ent.Name, other, symbol)
				}
				funcMangled[symbol] = ent.Name + ".конструктор"
			}
			if err := checkParamsAndBodyMangling(ent.Ctor.Params, ent.Ctor.Body); err != nil {
				return fmt.Errorf("сущность %q: конструктор: %w", ent.Name, err)
			}
		}
	}
	return nil
}

// checkParamsAndBodyMangling - параметры функции/метода и тело в общей
// корневой области видимости (как ctx.pushScope перед generateCallable в
// AST-пути), затем рекурсивно по вложенным блокам.
func checkParamsAndBodyMangling(params []ir.Param, body []ir.Stmt) error {
	scope := map[string]string{}
	for _, p := range params {
		if err := declareMangledVar(scope, p.Name); err != nil {
			return err
		}
	}
	return checkBlockManglingIn(scope, body)
}

// checkBlockMangling проверяет тело вложенного блока (если/иначе/пока/для) в
// СВЕЖЕЙ области видимости - повторное объявление того же имени во
// вложенном блоке разрешено (затенение, как в C), поэтому каждый блок
// начинается с пустой карты, а не наследует внешнюю.
func checkBlockMangling(body []ir.Stmt) error {
	return checkBlockManglingIn(map[string]string{}, body)
}

func checkBlockManglingIn(scope map[string]string, body []ir.Stmt) error {
	for _, st := range body {
		switch s := st.(type) {
		case *ir.VarDecl:
			if err := declareMangledVar(scope, s.Name); err != nil {
				return err
			}
		case *ir.IfStmt:
			if err := checkBlockMangling(s.Then); err != nil {
				return err
			}
			if err := checkBlockMangling(s.Else); err != nil {
				return err
			}
		case *ir.WhileStmt:
			if err := checkBlockMangling(s.Body); err != nil {
				return err
			}
		case *ir.GerundStmt:
			if err := checkBlockMangling(s.Body); err != nil {
				return err
			}
		case *ir.BlockStmt:
			if err := checkBlockMangling(s.Body); err != nil {
				return err
			}
		case *ir.ForStmt:
			loopScope := map[string]string{}
			if err := declareMangledVar(loopScope, s.Var); err != nil {
				return err
			}
			if err := checkBlockManglingIn(loopScope, s.Body); err != nil {
				return err
			}
		case *ir.ForEachStmt:
			loopScope := map[string]string{}
			if err := declareMangledVar(loopScope, s.Var); err != nil {
				return err
			}
			if err := checkBlockManglingIn(loopScope, s.Body); err != nil {
				return err
			}
		case *ir.CStyleForStmt:
			loopScope := map[string]string{}
			if s.Init != nil {
				if err := declareMangledVar(loopScope, s.Init.Name); err != nil {
					return err
				}
			}
			if err := checkBlockManglingIn(loopScope, s.Body); err != nil {
				return err
			}
		}
	}
	return nil
}

func declareMangledVar(scope map[string]string, name string) error {
	mangled := mangleVar(name)
	if other, exists := scope[mangled]; exists && other != name {
		return fmt.Errorf("переменные %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одну из них", other, name, mangled)
	}
	scope[mangled] = name
	return nil
}
