package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// exportAttr - префикс C-атрибута видимости перед функцией, соответствующей
// экспортированной Rech-функции, при сборке библиотеки (см. package doc
// generateSource) - "" во всех остальных случаях (обычная программа, или
// функция без "экспортировать"). Без него линковка в разделяемую
// библиотеку сделала бы видимым снаружи КАЖДЫЙ символ программы, а не
// только помеченные "экспортировать" - internal/toolchain.CC добавляет
// "-fvisibility=hidden" при сборке библиотеки на Linux/macOS именно ради
// этого атрибута (по умолчанию скрыто, явно - видимо). GCC/Clang-
// специфичный синтаксис - оба компилятора, которые ищет internal/toolchain
// (cc/clang/gcc), его понимают; на Windows/PE атрибут видимости не имеет
// эффекта (MinGW и так экспортирует все глобальные символы DLL по
// умолчанию - см. package doc CC.CompileLib, известное ограничение), но
// синтаксически безвреден.
func exportAttr(ctx *irGenCtx, exported bool) string {
	if !ctx.libMode || !exported {
		return ""
	}
	return `__attribute__((visibility("default"))) `
}

// generateHeader строит текст C-заголовка (.h) для prog - непрозрачные
// typedef сущностей и прототипы функций, помеченных "экспортировать".
// Прототипы построены теми же cReturnTypeFor/cParamListIR, что и настоящие
// определения в generateSource -
// не переизобретённой отдельной парой функций - иначе объявление в .h могло
// бы разойтись с реальной сигнатурой в .c при будущем изменении одной из
// сторон. Собственный минимальный ctx (только entities - используется
// fromIRType(...).cType для entityIsRef) - остальные флаги ctx (usesGC,
// usesShow и т. п.) заголовку не нужны, он не генерирует тела функций.
//
// Заголовок не переиспользует ни один текстовый блок из .c - внутренний
// рантайм компилятора (настоящие поля сущности, vtable, рантайм-хелперы) -
// деталь реализации, не часть публичного API, и никогда не должен быть
// виден потребителю библиотеки. typedef сущности в заголовке - неполное
// объявление структуры без тела ("typedef struct rech_T_Имя rech_T_Имя;"),
// тогда как в самом .c та же сущность - анонимная структура с тем же
// именем typedef (generateIREntityStruct) - два разных C-типа с одинаковым
// именем в двух разных единицах трансляции, без конфликта: потребитель
// заголовка получает и передаёт только указатель, никогда не разыменовывает
// его напрямую (тот же принцип непрозрачного дескриптора, что у FILE* в
// стандартной библиотеке C).
func generateHeader(prog *ir.Program) (string, error) {
	entities := map[string]*ir.Entity{}
	exportedEntityNames := map[string]bool{}
	for _, ent := range prog.Entities {
		entities[ent.Name] = ent
		if ent.Exported {
			exportedEntityNames[ent.Name] = true
		}
	}
	exportedFuncNames := map[string]bool{}
	for _, fn := range prog.Functions {
		if fn.Exported {
			exportedFuncNames[fn.Name] = true
		}
	}
	ctx := &irGenCtx{entities: entities, exportedEntityNames: exportedEntityNames, exportedFuncNames: exportedFuncNames}

	var typedefs strings.Builder
	for _, ent := range prog.Entities {
		if !ent.Exported {
			continue
		}
		fmt.Fprintf(&typedefs, "typedef struct %s %s;\n", entityCName(ctx, ent.Name), entityCName(ctx, ent.Name))
	}

	var protos strings.Builder
	for _, fn := range prog.Functions {
		if !fn.Exported {
			continue
		}
		fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType), funcCName(ctx, fn.Name), cParamListIR(ctx, fn.Params))
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech (rech build --lib). Не редактировать руками.\n")
	sb.WriteString("#ifndef RECH_LIBRARY_H\n#define RECH_LIBRARY_H\n\n")
	sb.WriteString("#include <stdint.h>\n#include <stdbool.h>\n\n")
	sb.WriteString("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n")
	if typedefs.Len() > 0 {
		sb.WriteString(typedefs.String())
		sb.WriteString("\n")
	}
	sb.WriteString(protos.String())
	sb.WriteString("\n#ifdef __cplusplus\n}\n#endif\n\n#endif\n")
	return sb.String(), nil
}
