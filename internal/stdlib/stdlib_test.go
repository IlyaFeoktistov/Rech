package stdlib_test

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
	"github.com/IlyaFeoktistov/Rech/internal/stdlib"
)

// core - встроенные имена, доступные без импорта. Новое встроенное имя
// обязано попасть либо сюда (осознанно), либо в модуль stdlib.Modules.
var core = map[string]bool{
	"прочитать_строку": true, "прочитать_целое": true, "прочитать_число": true, "выйти": true,
	"Вызываемый": true, "ОС": true,
}

func init() {
	for _, n := range append(append([]string{}, rtmsg.PanicKinds...), rtmsg.ErrorKinds...) {
		core[n] = true
	}
}

func TestEveryBuiltinIsCoreOrInExactlyOneModule(t *testing.T) {
	var all []string
	for _, f := range builtins.All {
		all = append(all, f.Name)
	}
	for n := range builtins.ReservedFuncNames {
		all = append(all, n)
	}
	for n := range builtins.ReservedEntityNames {
		all = append(all, n)
	}
	for n := range semantic.StandardEnums {
		all = append(all, n)
	}
	for _, n := range all {
		_, inModule := stdlib.ModuleOf(n)
		if inModule == core[n] {
			t.Errorf("%q: должно быть либо в ядре, либо в модуле (ядро=%v, модуль=%v)", n, core[n], inModule)
		}
	}
	seen := map[string]string{}
	for mod, names := range stdlib.Modules {
		for _, n := range names {
			if prev, dup := seen[n]; dup {
				t.Errorf("%q - в двух модулях: %s и %s", n, prev, mod)
			}
			seen[n] = mod
		}
	}
}

func TestImportLine(t *testing.T) {
	if got := stdlib.ImportLine("знак"); got != "использовать знак из стандарт/математика" {
		t.Fatalf("получено %q", got)
	}
	if stdlib.ImportLine("прочитать_строку") != "" {
		t.Fatal("ядро не импортируется")
	}
	if m, ok := stdlib.SplitPath("стандарт/бд"); !ok || m != "бд" {
		t.Fatalf("SplitPath: %q %v", m, ok)
	}
}
