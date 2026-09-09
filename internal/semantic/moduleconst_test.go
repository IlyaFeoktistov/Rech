package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeModuleConstVisibleFromAnyFunctionAndExplicitMain - константа
// уровня модуля (вне функции/сущности) видна из ЛЮБОЙ функции того же
// файла по голому имени, включая объявленную ВЫШЕ по файлу, чем сама
// константа (порядок не важен) - и, что раньше вообще не компилировалось,
// коэкзистирует с ЯВНОЙ "функция главная" без ошибки "программа объявляет
// функцию 'главная' явно и одновременно содержит инструкции на верхнем
// уровне" (константа - ОБЪЯВЛЕНИЕ, не инструкция неявной "главная").
func TestAnalyzeModuleConstVisibleFromAnyFunctionAndExplicitMain(t *testing.T) {
	prog := parseProgram(t, `
функция раньше(): Целое { вернуть ПОЗЖЕ }
константа ПОЗЖЕ = 5
функция главная { вывести раньше() }
`)
	fn := prog.Functions[0]
	ret := fn.Body[0].(*ir.ReturnStmt)
	lit, ok := ret.Value.(*ir.IntLiteral)
	if !ok || lit.Value != 5 {
		t.Fatalf("ожидался IntLiteral(5), получено %+v", ret.Value)
	}
}

func TestAnalyzeModuleConstDuplicateNameRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
константа А = 1
константа А = 2
функция главная { вывести А }
`)
	if err == nil || !strings.Contains(err.Error(), "объявлена более одного раза") {
		t.Fatalf("ожидалась ошибка про повторное объявление константы, получено: %v", err)
	}
}

func TestAnalyzeModuleConstSelfReferenceCycleRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
константа А = Б + 1
константа Б = А + 1
функция главная { вывести А }
`)
	if err == nil || !strings.Contains(err.Error(), "ссылается сама на себя") {
		t.Fatalf("ожидалась ошибка про циклическую ссылку константы, получено: %v", err)
	}
}

// TestAnalyzeModuleConstVisibleInsideGenericFunction - константа уровня
// модуля видна и внутри тела generic-функции/из места её вызова:
// instantiateGenerics (internal/semantic/generics.go) строит НОВЫЙ
// ast.Program для развёрнутых инстанциаций - раньше он не копировал
// prog.Consts, из-за чего константа "исчезала" ровно в файлах, где
// вообще есть хоть один generic (см. examples/бд/чанкование.rech).
func TestAnalyzeModuleConstVisibleInsideGenericFunction(t *testing.T) {
	prog := parseProgram(t, `
константа РАЗМЕР = 3
функция обернуть<T>(значение: T): T { вернуть значение }
функция главная { вывести обернуть<Целое>(РАЗМЕР) }
`)
	var fn *ir.Function
	for _, f := range prog.Functions {
		if f.Name == "главная" {
			fn = f
			break
		}
	}
	if fn == nil {
		t.Fatalf("функция \"главная\" не найдена среди %+v", prog.Functions)
	}
	print := fn.Body[0].(*ir.PrintStmt)
	call, ok := print.Value.(*ir.CallExpr)
	if !ok || len(call.Args) != 1 {
		t.Fatalf("ожидался вызов обернуть<Целое>(РАЗМЕР) с одним аргументом, получено %+v", print.Value)
	}
	lit, ok := call.Args[0].(*ir.IntLiteral)
	if !ok || lit.Value != 3 {
		t.Fatalf("ожидался IntLiteral(3) как аргумент (константа РАЗМЕР инлайнится), получено %+v", call.Args[0])
	}
}

func TestAnalyzeModuleConstNameCollisionWithFunctionRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция особая() { вывести "фн" }
константа особая = 5
функция главная { вывести особая }
`)
	if err == nil || !strings.Contains(err.Error(), "совпадает с уже объявленной функцией") {
		t.Fatalf("ожидалась ошибка про коллизию имени с функцией, получено: %v", err)
	}
}
