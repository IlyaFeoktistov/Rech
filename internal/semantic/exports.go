package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// validateExports проверяет ограничения "экспортировать" на уже готовом
// IR - типы параметров и принадлежность сущности к "структура"/"сущность"
// к этому моменту разрешены до конца, можно смотреть на них напрямую, не
// переоткрывая разрешение имён заново.
func validateExports(prog *ir.Program) error {
	exportedEntities := map[string]bool{}
	for _, ent := range prog.Entities {
		if !ent.Exported {
			continue
		}
		if ent.IsStruct {
			return errNoPos(CodeExportSemantic, "сущность %q: \"экспортировать\" пока не поддержан для \"структура\" - только для \"сущность\"/\"класс\" (ссылочная семантика, стабильное C-представление - указатель Boehm GC)", ent.Name)
		}
		exportedEntities[ent.Name] = true
	}

	for _, fn := range prog.Functions {
		if !fn.Exported {
			continue
		}
		for _, p := range fn.Params {
			if err := checkExportableType(p.Type, exportedEntities); err != nil {
				return diagnostics.Wrap(err, "экспортируемая функция %q: параметр %q", fn.Name, p.Name)
			}
		}
		if fn.HasReturn {
			if err := checkExportableType(fn.ReturnType, exportedEntities); err != nil {
				return diagnostics.Wrap(err, "экспортируемая функция %q: тип результата", fn.Name)
			}
		}
	}

	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if !m.Exported {
				continue
			}
			if !ent.Exported {
				return errNoPos(CodeExportSemantic, "сущность %q: метод %q помечен \"экспортировать\", но сама сущность - нет (пометьте \"экспортировать\" и перед %q)", ent.Name, m.Name, ent.Name)
			}
			for _, p := range m.Params {
				if err := checkExportableType(p.Type, exportedEntities); err != nil {
					return diagnostics.Wrap(err, "экспортируемый метод %q сущности %q: параметр %q", m.Name, ent.Name, p.Name)
				}
			}
			if m.HasReturn {
				if err := checkExportableType(m.ReturnType, exportedEntities); err != nil {
					return diagnostics.Wrap(err, "экспортируемый метод %q сущности %q: тип результата", m.Name, ent.Name)
				}
			}
		}

		if ent.Ctor != nil && ent.Ctor.Exported {
			if !ent.Exported {
				return errNoPos(CodeExportSemantic, "сущность %q: конструктор помечен \"экспортировать\", но сама сущность - нет (пометьте \"экспортировать\" и перед %q)", ent.Name, ent.Name)
			}
			for _, p := range ent.Ctor.Params {
				if err := checkExportableType(p.Type, exportedEntities); err != nil {
					return diagnostics.Wrap(err, "экспортируемый конструктор сущности %q: параметр %q", ent.Name, p.Name)
				}
			}
		}
	}
	return nil
}

// checkExportableType - t допустим в сигнатуре экспортируемой функции: один
// из примитивов со стабильным C-представлением (Целое/Действительное/
// Дробное/Логическое/Байт/Символ), Строка, или сущность, сама помеченная
// "экспортировать" (иначе граница ABI была бы неявной - тип оказался бы
// виден снаружи без того, чтобы это было осознанным решением на самой
// сущности). Файл/Сокет/Задача/Вызываемый/интерфейс/generic-коллекция - без
// стабильного внешнего представления, отклоняются понятной ошибкой, а не
// C-типом "void *" наугад.
func checkExportableType(t ir.Type, exportedEntities map[string]bool) error {
	switch t.Kind {
	case ir.KindByte, ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindChar, ir.KindBool, ir.KindString:
		return nil
	case ir.KindEntity:
		if exportedEntities[t.Entity] {
			return nil
		}
		return errNoPos(CodeExportSemantic, "тип %q используется в сигнатуре экспортируемой функции, но сама сущность не помечена \"экспортировать\"", t.Entity)
	default:
		return errNoPos(CodeExportSemantic, "тип %q недопустим в сигнатуре экспортируемой функции - нет стабильного C-представления на границе библиотеки", t.String())
	}
}
