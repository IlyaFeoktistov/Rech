package module

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// TestLinkModuleVarsImportAliasAndPrivacy - переменная модуля
// импортируется как функция, в том числе под псевдонимом: чтение,
// присваивание, "++" через псевдоним переписываются на настоящее имя;
// приватная и неимпортированная - ошибка видимости.
func TestLinkModuleVarsImportAliasAndPrivacy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "настройки.rech", "публичная переменная уровень = 3\nприватная переменная секрет = 1\nЦелое счётчик = 0\n")
	entry := writeFile(t, dir, "главный.rech", "использовать уровень, счётчик как сч из \"настройки\"\n\nфункция главная\n{\n    сч++\n    сч = сч + уровень\n}\n")
	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Vars) != 3 {
		t.Fatalf("ожидалось 3 переменных модуля в слитой программе, получено %d", len(merged.Vars))
	}
	body := merged.Functions[0].Body
	var names []string
	for _, s := range body {
		names = append(names, stmtTargetName(s))
	}
	if strings.Join(names, ",") != "счётчик,счётчик" {
		t.Fatalf("псевдоним должен переписаться на настоящее имя, получено %v", names)
	}

	priv := writeFile(t, dir, "приватный.rech", "использовать секрет из \"настройки\"\n\nфункция главная\n{\n}\n")
	if _, err := Load(priv, testFrontend{}); err == nil || !strings.Contains(err.Error(), "приватное") {
		t.Fatalf("ожидалась ошибка приватности, получено %v", err)
	}
	notImported := writeFile(t, dir, "без_импорта.rech", "использовать уровень из \"настройки\"\n\nфункция главная\n{\n    вывести счётчик\n}\n")
	p2, err := Load(notImported, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := p2.Link(); err == nil || !strings.Contains(err.Error(), "не импортирована") {
		t.Fatalf("ожидалась ошибка неимпортированной переменной, получено %v", err)
	}
}

// stmtTargetName - имя переменной, которую меняет инструкция ("x++" или
// "x = ...").
func stmtTargetName(s ast.Stmt) string {
	switch s := s.(type) {
	case *ast.ExprStmt:
		if s.IncDec != nil {
			return s.IncDec.Name
		}
	case *ast.Assign:
		return s.Name
	}
	return ""
}
