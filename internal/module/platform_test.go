package module

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// TestLoadResolvesPlatformSpecificFile - "использовать X из "путь""
// резолвится в "путь_<текущая платформа>.rech" (docs/SPEC.md), если такой
// файл лежит рядом, ЦЕЛИКОМ ВМЕСТО "путь.rech" (взаимозаменяемо, не
// слияние - см. package doc platformSpecificPath) - обычный "путь.rech"
// не читается вовсе, когда платформенный файл найден (в этом тесте он и
// не существует - если бы резолвинг ошибочно пошёл по нему, Load
// вернул бы ошибку "не удалось прочитать файл").
func TestLoadResolvesPlatformSpecificFile(t *testing.T) {
	dir := t.TempDir()
	current := ast.CurrentPlatform()
	writeFile(t, dir, "окно_"+strings.ToLower(current)+".rech", "публичная функция открыть(): Строка\n{\n    вернуть \""+current+"\"\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать открыть из \"окно\"\n\nфункция главная\n{\n    вывести открыть()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(prog.Modules) != 2 {
		t.Fatalf("ожидалось 2 модуля, получено %d", len(prog.Modules))
	}
}
