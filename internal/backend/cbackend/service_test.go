package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestServiceProgramWrapsEntryPoint - у службы тело "главная" - отдельная
// функция, main() - диспетчер служб Windows с запасным консольным путём, и
// флаг остановки есть, даже если программа его не опрашивает.
func TestServiceProgramWrapsEntryPoint(t *testing.T) {
	prog := &ir.Program{Service: true, Functions: []*ir.Function{{Name: entryPointName}}}
	src, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("GenerateFromIR: %v", err)
	}
	for _, want := range []string{
		"int rech_service_body(int argc, char **argv) {",
		"StartServiceCtrlDispatcherW(table)",
		"ERROR_FAILED_SERVICE_CONTROLLER_CONNECT",
		"static volatile sig_atomic_t rech_shutdown_flag",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("нет %q в коде службы", want)
		}
	}
	if strings.Count(src, "int main(int argc, char **argv) {") != 2 {
		t.Errorf("ожидалось два main() - для Windows и остальных ОС")
	}
}
