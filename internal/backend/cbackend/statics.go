// Статические поля сущностей/структур ("статическое поле", ir.Entity.
// StaticFields) - хранилище одно на всю программу, не на экземпляр (в
// отличие от обычных полей - никогда не входит в C-структуру экземпляра,
// см. generateIREntityStruct). Представлены обычными C-глобальными
// переменными (staticFieldsDecl) с нулевым значением по умолчанию (та же
// гарантия C, что и у зануления GC_MALLOC'нутой памяти для обычных полей) -
// то, что задано явным "= выражение" (Field.Default), заполняется РОВНО
// один раз, до входа в "главная" (staticFieldsInitCode), rech_init_statics()
// вызывается из main() тем же принципом, что rech_init_console/
// rech_init_args (см. generateIRFunction).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// generateIRStaticVarDecl - "статическая переменная имя = выражение"
// (ir.VarDecl.IsStatic) - значение вычисляется и сохраняется РОВНО один раз
// за всё время работы программы (при первом достижении этой инструкции), как
// "static" локальная переменная в C: C гарантирует, что static-локальная
// сама по себе объявлена ровно один раз для данного лексического места в
// исходнике, независимо от того, сколько раз выполнение туда возвращается
// (повторный вызов функции, следующая итерация цикла) - этого достаточно
// само по себе (см. package doc ir.VarDecl.IsStatic - StaticID здесь не
// нужен, в отличие от internal/interp, где такой гарантии от языка Go нет).
// Явный флаг "уже инициализировано" нужен только потому, что C требует
// константное выражение для инициализатора объекта со статической
// длительностью хранения - <code> в общем случае им не является (см.
// package doc внизу файла), поэтому инициализация вынесена в "if" тела,
// исполняемый один раз. Временная "_INIT" переменная - та же защита от
// самоссылающегося затенения, что и у обычной ir.VarDecl (см.
// generateIRStmt, случай без IsStatic).
func generateIRStaticVarDecl(sb *strings.Builder, ctx *irGenCtx, s *ir.VarDecl, ind string) error {
	code, typ, err := generateIRExpr(ctx, s.Value)
	if err != nil {
		return err
	}
	cType := typ.cType(ctx)
	mangled := mangleVar(s.Name)
	inited := mangled + "_STATIC_INITED"
	tmp := mangled + "_INIT"
	fmt.Fprintf(sb, "%sstatic %s %s;\n", ind, cType, mangled)
	fmt.Fprintf(sb, "%sstatic bool %s = false;\n", ind, inited)
	fmt.Fprintf(sb, "%sif (!%s) {\n", ind, inited)
	fmt.Fprintf(sb, "%s    %s %s = %s;\n", ind, cType, tmp, code)
	fmt.Fprintf(sb, "%s    %s = %s;\n", ind, mangled, tmp)
	fmt.Fprintf(sb, "%s    %s = true;\n", ind, inited)
	fmt.Fprintf(sb, "%s}\n", ind)
	return nil
}

// staticFieldsDecl - "static Тип rech_S_Сущность_поле;" для каждого
// статического поля программы, в порядке объявления сущностей/полей. C
// гарантирует нулевую инициализацию объектов со статической длительностью
// хранения без явного инициализатора - полю без Field.Default этого
// достаточно, отдельный код не нужен.
func staticFieldsDecl(ctx *irGenCtx, prog *ir.Program) string {
	var sb strings.Builder
	for _, ent := range prog.Entities {
		for _, f := range ent.StaticFields {
			fmt.Fprintf(&sb, "static %s %s;\n", fromIRType(f.Type).cType(ctx), mangleStaticField(ent.Name, f.Name))
		}
	}
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

// staticFieldsInitCode - "static void rech_init_statics(void) { ... }",
// заполняющая явно заданные (Field.Default != nil) статические поля -
// вызывается из main() РОВНО один раз, до первой инструкции программы (см.
// generateIRFunction), поэтому, в отличие от "статическая переменная"
// внутри функции (см. package doc ir.VarDecl.IsStatic), не нуждается в
// собственном флаге "уже инициализировано".
func staticFieldsInitCode(ctx *irGenCtx, prog *ir.Program) (string, error) {
	var body strings.Builder
	for _, ent := range prog.Entities {
		for _, f := range ent.StaticFields {
			if f.Default == nil {
				continue
			}
			code, _, err := generateIRExpr(ctx, f.Default)
			if err != nil {
				return "", fmt.Errorf("сущность %q: статическое поле %q: %w", ent.Name, f.Name, err)
			}
			fmt.Fprintf(&body, "    %s = %s;\n", mangleStaticField(ent.Name, f.Name), code)
		}
	}
	if body.Len() == 0 {
		return "", nil
	}
	var sb strings.Builder
	sb.WriteString("static void rech_init_statics(void) {\n")
	sb.WriteString(body.String())
	sb.WriteString("}\n\n")
	return sb.String(), nil
}
