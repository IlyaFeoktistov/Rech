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

// cDocComment - Doc сущности/функции (уже очищенный от "/**"/"*/"/
// ведущих "*" текст, см. internal/parser.normalizeDoc) в виде настоящего
// C doc-комментария (Javadoc-стиль - тот же, что пользователь и так пишет
// в исходнике на Rech, просто восстановленный после normalizeDoc) - перед
// typedef/прототипом в сгенерированном .h, чтобы потребитель библиотеки
// увидел исходную документацию автора, не только голую сигнатуру. ""
// (пустой Doc - объявление без doc-комментария) - ничего не добавляется.
func cDocComment(doc string) string {
	if doc == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("/**\n")
	for line := range strings.SplitSeq(doc, "\n") {
		if line == "" {
			sb.WriteString(" *\n")
		} else {
			fmt.Fprintf(&sb, " * %s\n", line)
		}
	}
	sb.WriteString(" */\n")
	return sb.String()
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
	ctx := headerCtx(prog)

	var typedefs strings.Builder
	for _, ent := range prog.Entities {
		if !ent.Exported {
			continue
		}
		typedefs.WriteString(cDocComment(ent.Doc))
		fmt.Fprintf(&typedefs, "typedef struct %s %s;\n", entityCName(ctx, ent.Name), entityCName(ctx, ent.Name))
	}

	var protos strings.Builder
	for _, fn := range prog.Functions {
		if !fn.Exported {
			continue
		}
		protos.WriteString(cDocComment(fn.Doc))
		fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType), funcCName(ctx, fn.Name), cParamListIR(ctx, fn.Params))
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if !m.Exported {
				continue
			}
			protos.WriteString(cDocComment(m.Doc))
			fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), methodCName(ctx, ent.Name, m.Name), cMethodParamListIR(ctx, ent.Name, m.Params))
		}
		if ent.Ctor != nil && ent.Ctor.Exported {
			fmt.Fprintf(&protos, "%s *%s(%s);\n", entityCName(ctx, ent.Name), ctorCName(ctx, ent.Name), cParamListIR(ctx, ent.Ctor.Params))
		}
	}
	for _, g := range prog.Globals {
		if !g.Exported {
			continue
		}
		protos.WriteString(cDocComment(g.Doc))
		fmt.Fprintf(&protos, "extern %s %s;\n", fromIRType(g.Type).cType(ctx), g.Name)
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

// headerCtx - контекст имён для заголовка библиотеки и ExportedSymbols:
// какие сущности/функции/методы/конструкторы помечены "экспортировать".
func headerCtx(prog *ir.Program) *irGenCtx {
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
	exportedMethods := buildExportedMethods(prog)
	exportedCtors := buildExportedCtors(prog)
	return &irGenCtx{entities: entities, exportedEntityNames: exportedEntityNames, exportedFuncNames: exportedFuncNames, exportedMethods: exportedMethods, exportedCtors: exportedCtors}
}

// ExportedSymbols - C-имена всего, что объявляет заголовок библиотеки
// (функции, методы, конструкторы с "экспортировать"): при сборке
// статической библиотеки остальные символы архива делаются локальными.
func ExportedSymbols(prog *ir.Program) []string {
	ctx := headerCtx(prog)
	var names []string
	for _, fn := range prog.Functions {
		if fn.Exported {
			names = append(names, funcCName(ctx, fn.Name))
		}
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if m.Exported {
				names = append(names, methodCName(ctx, ent.Name, m.Name))
			}
		}
		if ent.Ctor != nil && ent.Ctor.Exported {
			names = append(names, ctorCName(ctx, ent.Name))
		}
	}
	for _, g := range prog.Globals {
		if g.Exported {
			names = append(names, g.Name)
		}
	}
	return names
}
