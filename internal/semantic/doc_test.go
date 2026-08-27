package semantic

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func findEntity(prog *ir.Program, name string) *ir.Entity {
	for _, ent := range prog.Entities {
		if ent.Name == name {
			return ent
		}
	}
	return nil
}

func findFunction(prog *ir.Program, name string) *ir.Function {
	for _, fn := range prog.Functions {
		if fn.Name == name {
			return fn
		}
	}
	return nil
}

func TestAnalyzeDocCommentsFlowIntoIR(t *testing.T) {
	src := `
/** Точка на плоскости. */
сущность Точка
{
    /** Предел координаты. */
    константа ПРЕДЕЛ = 100

    имеет:
        /** Горизонтальная координата. */
        x: Целое

    /** Расстояние до начала координат. */
    длина(): Целое
    {
        вернуть этот.x
    }
}

/** Складывает два числа. */
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}

функция главная { вывести "ок" }
`
	prog := parseProgram(t, src)

	ent := findEntity(prog, "Точка")
	if ent == nil {
		t.Fatalf("сущность Точка не найдена в IR")
	}
	if ent.Doc != "Точка на плоскости." {
		t.Fatalf("неожиданный Doc сущности: %q", ent.Doc)
	}
	if len(ent.Constants) != 1 || ent.Constants[0].Doc != "Предел координаты." {
		t.Fatalf("неожиданный Doc константы: %+v", ent.Constants)
	}
	if len(ent.Fields) != 1 || ent.Fields[0].Doc != "Горизонтальная координата." {
		t.Fatalf("неожиданный Doc поля: %+v", ent.Fields)
	}
	if len(ent.Methods) != 1 || ent.Methods[0].Doc != "Расстояние до начала координат." {
		t.Fatalf("неожиданный Doc метода: %+v", ent.Methods)
	}

	fn := findFunction(prog, "сложить")
	if fn == nil {
		t.Fatalf("функция сложить не найдена в IR")
	}
	if fn.Doc != "Складывает два числа." {
		t.Fatalf("неожиданный Doc функции: %q", fn.Doc)
	}
}
