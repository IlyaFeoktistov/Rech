// Динамическая диспетчеризация для наследования ("наследует") и
// интерфейсов ("реализует") - оба используют один и тот же механизм:
// единый указатель на таблицу функций (vt), встроенный первым скрытым
// полем в C-структуру сущности, но только у сущностей, реально
// участвующих в иерархии наследования или реализующих хотя бы один
// интерфейс (dispatchParticipants) - плоская сущность вне какой-либо
// иерархии/интерфейса продолжает вызываться напрямую по мангл-имени, без
// какого-либо дополнительного кода или накладных расходов.
//
// Каждому уникальному имени метода, встретившемуся хоть в одном
// интерфейсе или у участвующей в диспетчеризации сущности во всей
// программе, присваивается глобальный номер слота (dispatchSlots) -
// таблица конкретного типа заполняется указателями на его собственные/
// унаследованные функции по этим номерам; слот метода, которого у этого
// типа нет, остаётся нулевым - недостижимым, потому что вызвать его можно
// только там, где семантический анализ уже гарантировал соответствие
// интерфейсу/сигнатуре. Слоты хранятся как обобщённые указатели на
// функцию (rech_vtfunc, "void (*)(void)") и приводятся к настоящей
// сигнатуре только в точке вызова - тот же приём, которым в C реализуют
// полиморфизм без встроенной поддержки языка (аналог vtable в GObject/COM).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// vtfuncType/vtableType/vtableHeaderType - имена синтезированных C-типов
// диспетчеризации, общих на всю программу (не по одному на интерфейс/
// иерархию - один общий, проиндексированный глобальным номером слота).
const (
	vtfuncType       = "rech_vtfunc"
	vtableType       = "rech_vtable"
	vtableHeaderType = "rech_vtable_header"
)

func vtableInstanceName(entityName string) string {
	return "rech_VT_" + transliterate(entityName)
}

// computeDispatch - единственное место, где решается, какие сущности
// участвуют в диспетчеризации (dispatchParticipants) и какой глобальный
// номер слота получает каждое имя метода (dispatchSlots). Участвует
// сущность X, если: у X есть базовая ("наследует", X.BaseName != ""),
// ИЛИ X сама - чья-то базовая (наследуется хоть одной другой сущностью
// программы), ИЛИ X "реализует" хотя бы один интерфейс. Слоты собираются
// сначала по методам интерфейсов (в порядке объявления интерфейса), затем
// по собственным методам участвующих сущностей (в порядке prog.Entities)
// - имя, уже получившее слот, повторно не пересчитывается.
func computeDispatch(prog *ir.Program) (participants map[string]bool, slots map[string]int) {
	participants = map[string]bool{}
	isBase := map[string]bool{}
	for _, ent := range prog.Entities {
		if ent.BaseName != "" {
			participants[ent.Name] = true
			isBase[ent.BaseName] = true
		}
		if len(ent.Interfaces) > 0 {
			participants[ent.Name] = true
		}
	}
	for name := range isBase {
		participants[name] = true
	}

	slots = map[string]int{}
	next := 0
	assign := func(name string) {
		if _, ok := slots[name]; !ok {
			slots[name] = next
			next++
		}
	}
	for _, iface := range prog.Interfaces {
		for _, m := range iface.Methods {
			assign(m.Name)
		}
	}
	for _, ent := range prog.Entities {
		if !participants[ent.Name] {
			continue
		}
		for _, m := range ent.Methods {
			assign(m.Name)
		}
	}
	return participants, slots
}

// needsDispatch - true, если получатель статического типа name (сущность
// или интерфейс) должен вызываться через vtable, а не напрямую по
// мангл-имени - единственное место, где cbackend решает "прямой вызов или
// диспетчеризация" (generateIRMethodCallExpr).
func (ctx *irGenCtx) needsDispatch(name string) bool {
	if _, ok := ctx.interfaces[name]; ok {
		return true
	}
	return ctx.dispatchParticipants[name]
}

// vtableTypedefs - типы диспетчеризации, эмитятся один раз в начале файла,
// только если ctx.usesVtable (хотя бы одна сущность участвует в
// диспетчеризации где-нибудь в программе).
func vtableTypedefs(slotCount int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "typedef void (*%s)(void);\n", vtfuncType)
	// type_id - позиция типа в rech_isa (internal/backend/cbackend/
	// exceptions.go, buildIsaTable) - общий для наследования/интерфейсов
	// vtable уже даёт то же самое разделение "vt хранится один на КОНКРЕТНЫЙ
	// тип, не на объект" бесплатно: экземпляр отличается только этим полем,
	// не отдельным полем в самом объекте.
	fmt.Fprintf(&sb, "typedef struct { int type_id; %s slots[%d]; } %s;\n", vtfuncType, slotCount, vtableType)
	fmt.Fprintf(&sb, "typedef struct { const %s *vt; } %s;\n\n", vtableType, vtableHeaderType)
	return sb.String()
}

// generateVtableInstance строит статическую таблицу диспетчеризации
// конкретной участвующей сущности ent - по каждому известному программе
// слоту (ctx.dispatchSlots) ищет метод с этим именем сначала среди
// собственных методов ent, затем вверх по цепочке BaseName (findVtableSlotFunc)
// - неизменённый унаследованный метод не дублируется, слот просто
// указывает на C-функцию базовой сущности напрямую. Слот метода, которого
// нет ни у ent, ни у одного из её предков - не заполняется вовсе
// (designated initializer оставляет его нулевым) - вызвать его нельзя,
// семантический анализ такого не пропустит.
func generateVtableInstance(ctx *irGenCtx, ent *ir.Entity) string {
	var sb strings.Builder
	fmt.Fprintf(
		&sb,
		"static const %s %s = {\n    .type_id = %d,\n    .slots = {\n",
		vtableType,
		vtableInstanceName(ent.Name),
		ctx.excTypeIDs[ent.Name],
	)
	for name, slot := range ctx.dispatchSlots {
		definer := findVtableSlotFunc(ctx, ent.Name, name)
		if definer == "" {
			continue
		}
		fmt.Fprintf(&sb, "        [%d] = (%s)&%s,\n", slot, vtfuncType, methodCName(ctx, definer, name))
	}
	sb.WriteString("    }\n};\n\n")
	return sb.String()
}

// findVtableSlotFunc ищет, какая сущность (сама entityName или предок по
// BaseName) реально определяет метод name - "" если ни одна. Абстрактный
// метод (m.IsAbstract, без тела и без сгенерированной C-функции - см.
// package doc ir.Method.IsAbstract) не считается определением - поиск идёт
// дальше вверх по цепочке, как если бы этой сущности метод вовсе не
// объявляла. Для сущности, реально СОЗДАВАЕМОЙ в программе, дальше по
// цепочке гарантированно найдётся настоящая реализация (internal/semantic/
// analyzer.go отклоняет иначе); для самой абстрактной сущности (её
// собственная таблица диспетчеризации никогда не используется - она не
// инстанциируется) слот просто останется незаполненным.
func findVtableSlotFunc(ctx *irGenCtx, entityName, name string) string {
	for entityName != "" {
		ent, ok := ctx.entities[entityName]
		if !ok {
			return ""
		}
		for _, m := range ent.Methods {
			if m.Name == name && !m.IsAbstract {
				return entityName
			}
		}
		entityName = ent.BaseName
	}
	return ""
}

// generateDispatchCall строит вызов метода через vtable - получатель
// (receiverCode, уже вычисленный) приводится к vtableHeaderType, чтобы
// прочитать его "vt" (первое поле любой участвующей сущности - общий
// начальный кусок структуры делает этот приём определённым для чтения),
// слот с номером ctx.dispatchSlots[e.Method] приводится к настоящей
// сигнатуре вызова, взятой из самого MethodCallExpr (e.Type/e.HasReturn
// для результата, TypeOf каждого аргумента для параметров - этого
// достаточно, отдельная сигнатура метода интерфейса/сущности здесь не
// нужна). receiverCode безопасно встретится в тексте дважды - получатель
// вызова метода семантически ограничен переменной/полем/"этот"
// (receiverAddressable), без побочных эффектов при повторном вычислении.
func generateDispatchCall(ctx *irGenCtx, e *ir.MethodCallExpr, receiverCode string, argCodes []string) (string, valueType, error) {
	slot, ok := ctx.dispatchSlots[e.Method]
	if !ok {
		return "", typeUnknown, fmt.Errorf("генерация C из IR: метод %q не имеет слота диспетчеризации", e.Method)
	}
	retC := "int"
	if e.HasReturn {
		retC = fromIRType(e.Type).cType(ctx)
	}
	paramTypes := make([]string, len(e.Args)+1)
	paramTypes[0] = "void *"
	for i, a := range e.Args {
		paramTypes[i+1] = fromIRType(ir.TypeOf(a)).cType(ctx)
	}
	fnType := fmt.Sprintf("%s (*)(%s)", retC, strings.Join(paramTypes, ", "))
	call := fmt.Sprintf("((%s)((const %s *)(%s))->vt->slots[%d])(%s)",
		fnType, vtableHeaderType, receiverCode, slot, strings.Join(argCodes, ", "))
	return call, fromIRType(e.Type), nil
}
