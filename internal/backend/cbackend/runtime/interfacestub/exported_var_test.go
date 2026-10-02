package interfacestub

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateExportedModuleVar - экспортированная переменная модуля в
// .rechi - внешняя переменная под тем же именем.
func TestGenerateExportedModuleVar(t *testing.T) {
	out, err := Generate(&ir.Program{Globals: []*ir.GlobalVar{
		{Name: "создано", Type: ir.TypeInt, Exported: true},
		{Name: "внутреннее", Type: ir.TypeInt},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "внешняя Целое создано\n") || strings.Contains(out, "внутреннее") {
		t.Fatalf("ожидалась только \"внешняя Целое создано\", получено:\n%s", out)
	}
}
