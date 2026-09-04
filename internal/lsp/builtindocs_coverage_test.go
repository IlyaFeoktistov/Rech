package lsp

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

// TestHoverBuiltinFuncCoversAllReservedNames - каждое имя из internal/
// builtins.All и ReservedFuncNames должно давать непустой hover - находка
// по прямой просьбе пользователя ("другие функции тоже не задокументированы
// в док-блоках"): "сетевой_узел" (заменил старые "слушать_tcp"/
// "подключиться_tcp"/"подключиться_udp", см. package doc builtins.go),
// "получить_переменную_окружения" и несколько других не имели записи вовсе,
// "принять" был в НЕВЕРНОЙ карте (builtinFuncShortDoc - читается только
// внутри ветки builtins.ByName, а "принять" не входит в builtins.All, только
// в ReservedFuncNames) - hover для него не показывал ничего, несмотря на
// формально существующую строку описания.
func TestHoverBuiltinFuncCoversAllReservedNames(t *testing.T) {
	for _, f := range builtins.All {
		if _, ok := hoverBuiltinFunc(f.Name); !ok {
			t.Errorf("нет hover для встроенной функции %q (builtins.All)", f.Name)
		}
	}
	for name := range builtins.ReservedFuncNames {
		if _, ok := hoverBuiltinFunc(name); !ok {
			t.Errorf("нет hover для зарезервированной функции %q (ReservedFuncNames)", name)
		}
	}
}

// TestHoverBuiltinFuncNetworkNode - "сетевой_узел" (единая замена трёх
// старых функций) явно показывает сигнатуру с примитивными типами.
func TestHoverBuiltinFuncNetworkNode(t *testing.T) {
	text, ok := hoverBuiltinFunc("сетевой_узел")
	if !ok {
		t.Fatalf("ожидался hover \"сетевой_узел\"")
	}
	if !strings.Contains(text, "сетевой_узел(Строка, Строка, Целое): Сокет") {
		t.Fatalf("неожиданная сигнатура: %q", text)
	}
}

// TestHoverBuiltinFuncAccept - "принять" (ReservedFuncNames, не
// builtins.All - см. package doc теста выше) показывает полную сигнатуру
// с nullable-возвратом.
func TestHoverBuiltinFuncAccept(t *testing.T) {
	text, ok := hoverBuiltinFunc("принять")
	if !ok {
		t.Fatalf("ожидался hover \"принять\"")
	}
	if !strings.Contains(text, "принять(сокет: Сокет): ?Сокет") {
		t.Fatalf("неожиданная сигнатура: %q", text)
	}
}
