package semantic

import (
	"fmt"

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
			return fmt.Errorf("сущность %q: \"экспортировать\" пока не поддержан для \"структура\" - только для \"сущность\"/\"класс\" (ссылочная семантика, стабильное C-представление - указатель Boehm GC)", ent.Name)
		}
		exportedEntities[ent.Name] = true
	}

	for _, fn := range prog.Functions {
		if !fn.Exported {
			continue
		}
		for _, p := range fn.Params {
			if err := checkExportableType(p.Type, exportedEntities); err != nil {
				return fmt.Errorf("экспортируемая функция %q: параметр %q: %w", fn.Name, p.Name, err)
			}
		}
		if fn.HasReturn {
			if err := checkExportableType(fn.ReturnType, exportedEntities); err != nil {
				return fmt.Errorf("экспортируемая функция %q: тип результата: %w", fn.Name, err)
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
		return fmt.Errorf("тип %q используется в сигнатуре экспортируемой функции, но сама сущность не помечена \"экспортировать\"", t.Entity)
	default:
		return fmt.Errorf("тип %q недопустим в сигнатуре экспортируемой функции - нет стабильного C-представления на границе библиотеки", t.String())
	}
}
