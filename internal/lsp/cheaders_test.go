package lsp

import "testing"

func TestExtractDeclCommentBlockComment(t *testing.T) {
	src := `
extern int unrelated(void);

/* Return the length of S.  */
extern size_t strlen (const char *__s) __THROW;
`
	got := extractDeclComment(src, "strlen", true)
	if got != "Return the length of S." {
		t.Fatalf("неожиданный текст: %q", got)
	}
}

// C-имена ВСЕГДА ASCII (валидный C-идентификатор) - тестовые cSymbol
// намеренно латиницей, в отличие от текста самого комментария (может быть
// каким угодно, в т. ч. кириллицей). Go regexp (RE2) \b - ASCII-only, на
// кириллице "\bИмя\b" не сработал бы вовсе (тот же класс проблемы, что и у
// "\w" в internal/interp/regex.go/internal/backend/cbackend/regex.go, см.
// их package doc) - не баг конкретно здесь, просто область входа уже
// сужена самим C.
func TestExtractDeclCommentLineCommentRun(t *testing.T) {
	src := `
// Складывает два числа.
// Возвращает результат.
int add(int a, int b);
`
	got := extractDeclComment(src, "add", true)
	if got != "Складывает два числа. Возвращает результат." {
		t.Fatalf("неожиданный текст: %q", got)
	}
}

func TestExtractDeclCommentNoneWhenNoCommentDirectlyAbove(t *testing.T) {
	src := `
/* Комментарий про совсем другое. */
int junk(void);

int add(int a, int b);
`
	got := extractDeclComment(src, "add", true)
	if got != "" {
		t.Fatalf("не ожидался комментарий чужой декларации: %q", got)
	}
}

// TestExtractDeclCommentSkipsEarlierUncommentedOccurrence - живой баг,
// найденный на настоящем glibc string.h: "strlen" встречается первым внутри
// тела чужого макроса ("strdupa"), без комментария - настоящее объявление,
// с комментарием прямо над ним, идёт ВТОРЫМ. Первая версия extractDeclComment
// проверяла только самое первое вхождение и возвращала "" уже на нём, даже
// не дойдя до настоящего объявления.
func TestExtractDeclCommentSkipsEarlierUncommentedOccurrence(t *testing.T) {
	src := `
#define strdupa(s) (strlen(s) + 1)

/* Return the length of S.  */
extern size_t strlen (const char *__s) __THROW;
`
	got := extractDeclComment(src, "strlen", true)
	if got != "Return the length of S." {
		t.Fatalf("неожиданный текст: %q", got)
	}
}

func TestExtractDeclCommentNotFound(t *testing.T) {
	src := `int сложить(int a, int b);`
	if got := extractDeclComment(src, "вычесть", true); got != "" {
		t.Fatalf("не ожидалось совпадение: %q", got)
	}
}

func TestExtractDeclCommentOpaqueTypeWithoutCall(t *testing.T) {
	src := `
/* Непрозрачный дескриптор блока памяти. */
typedef void *BLOCK;
`
	got := extractDeclComment(src, "BLOCK", false)
	if got != "Непрозрачный дескриптор блока памяти." {
		t.Fatalf("неожиданный текст: %q", got)
	}
}

func TestCleanCComment(t *testing.T) {
	cases := map[string]string{
		"/* single line */":           "single line",
		"/**\n * multi\n * line\n */": "multi line",
		"// один // два":              "один // два",
	}
	for raw, want := range cases {
		if got := cleanCComment(raw); got != want {
			t.Fatalf("cleanCComment(%q) = %q, ожидалось %q", raw, got, want)
		}
	}
}
