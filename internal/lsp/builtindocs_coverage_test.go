package lsp

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// TestHoverCoversAllReservedEntityNames - каждое имя из internal/
// builtins.ReservedEntityNames должно давать непустой hover
// (builtinTypeDocs, internal/lsp/builtindocs.go) - репортнуто пользователем
// ("подсказки hover есть не на всех новых функциях и сущностях"): каждая
// зарезервированная сущность, добавленная позже первоначального списка
// (RGBЦвет/ДекодированноеИзображение/Процесс/РезультатКоманды - Шаги про
// картинки/внешние процессы, Дейтаграмма/ОпцииКуки/Совпадение/HTTP2Поток -
// более ранние сетевые шаги) не получила записи в builtinTypeDocs вовсе -
// та же забывчивость, что уже однажды поймал сам компилятор для
// "RGBЦвет" на уровне ReservedEntityNames (internal/builtins/builtins.go,
// комментарий там же), только уровнем выше - hover.
func TestHoverCoversAllReservedEntityNames(t *testing.T) {
	d := newDoc("test://a.rech", "функция главная\n{\n}\n")
	for name := range builtins.ReservedEntityNames {
		if name == "Паника" {
			continue // псевдо-тип, распознаётся только в "поймать (x: Паника)" - не настоящая сущность, hover для голого имени не ожидается
		}
		if _, ok := d.hoverBareName(name, 1); !ok {
			t.Errorf("нет hover для зарезервированной сущности %q (builtins.ReservedEntityNames)", name)
		}
	}
}

// TestHoverKeywordCoversAllTokenTypes - каждое ключевое слово языка (internal/
// lexer.KeywordTokenTypes) должно иметь запись в keywordDocs - находка при
// сборке справочника документации ("документацию тоже надо сверить, вдруг
// что не описали"): "статический"/"выбрать"/"когда"/"YAML" не имели записи
// вовсе, несмотря на то что все четыре - реализованные, рабочие фичи
// (статические поля/методы, switch, сериализация YAML).
func TestHoverKeywordCoversAllTokenTypes(t *testing.T) {
	for _, tt := range lexer.KeywordTokenTypes() {
		if _, ok := keywordDocs[tt]; !ok {
			t.Errorf("нет hover-документации для ключевого слова с TokenType %v", tt)
		}
	}
}

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
	if !strings.Contains(text, "сетевой_узел(Целое, Строка, Целое): Сокет") {
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
