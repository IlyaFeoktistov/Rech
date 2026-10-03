package lexer

import (
	"errors"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

func TestLexerLineCommentUnaffectedByBlockCommentAddition(t *testing.T) {
	toks, err := New("переменная x = 1 // комментарий\nвывести x").Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, tok := range toks {
		if tok.Type == DOC_COMMENT {
			t.Fatalf("\"//\" не должен стать DOC_COMMENT: %+v", toks)
		}
	}
}

// TestLexerBlockCommentIsSkippedLikeWhitespace - "/* ... */" (в т. ч. на
// несколько строк) не порождает ни одного токена, включая NEWLINE - весь
// перенос строки внутри комментария поглощается вместе с ним.
func TestLexerBlockCommentIsSkippedLikeWhitespace(t *testing.T) {
	types := tokenTypes(t, "переменная /* коммент\nна две строки */ x = 1")
	want := []TokenType{KwVariable, IDENT, ASSIGN, INT, EOF}
	if len(types) != len(want) {
		t.Fatalf("получено %v, ожидалось %v", types, want)
	}
	for i, tt := range want {
		if types[i] != tt {
			t.Fatalf("токен %d: получено %v, ожидалось %v (все токены: %v)", i, types[i], tt, types)
		}
	}
}

// TestLexerEmptyBlockCommentIsNotDoc - "/**/" (закрывается немедленно) -
// обычный пустой блочный комментарий, не doc-комментарий.
func TestLexerEmptyBlockCommentIsNotDoc(t *testing.T) {
	types := tokenTypes(t, "/**/переменная x = 1")
	if len(types) == 0 || types[0] != KwVariable {
		t.Fatalf("\"/**/\" должен быть поглощён как пробел, получено %v", types)
	}
}

// TestLexerDocCommentBecomesToken - "/** текст */" становится токеном
// DOC_COMMENT с текстом между "/**" и "*/" как есть.
func TestLexerDocCommentBecomesToken(t *testing.T) {
	toks, err := New("/** Складывает два числа. */\nфункция сложить").Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if toks[0].Type != DOC_COMMENT {
		t.Fatalf("ожидался DOC_COMMENT первым токеном, получено %v", toks[0].Type)
	}
	if toks[0].Value != " Складывает два числа. " {
		t.Fatalf("неожиданный текст doc-комментария: %q", toks[0].Value)
	}
}

func TestLexerUnclosedBlockCommentIsError(t *testing.T) {
	if _, err := New("переменная x /* не закрыт").Tokenize(); err == nil {
		t.Fatalf("ожидалась ошибка на незакрытом блочном комментарии")
	}
}

func TestLexerUnclosedDocCommentIsError(t *testing.T) {
	if _, err := New("/** не закрыт").Tokenize(); err == nil {
		t.Fatalf("ожидалась ошибка на незакрытом doc-комментарии")
	}
}

// TestLexerCommentInTemplateExprIsError - вставка "${...}" - часть строки:
// "//" и "/*" в ней - ошибка с позицией в исходнике всего файла, а не тихо
// выброшенный остаток выражения. Вне вставки "//" - обычный текст строки.
func TestLexerCommentInTemplateExprIsError(t *testing.T) {
	for _, src := range []string{"вывести `${1 // 2}`", "вывести `${1 /* 2 */}`"} {
		toks, err := New(src).Tokenize()
		if err != nil {
			t.Fatalf("%s: неожиданная ошибка лексера: %v", src, err)
		}
		_, err = TemplatePartTokens(toks[1].Parts[0])
		var d *diagnostics.Diagnostic
		if !errors.As(err, &d) || d.Code != CodeCommentInTemplate {
			t.Fatalf("%s: ожидалась ошибка %s, получено %v", src, CodeCommentInTemplate, err)
		}
		if d.Pos.Line != 1 || d.Pos.Col != 14 {
			t.Fatalf("%s: позиция %d:%d, ожидалось 1:14", src, d.Pos.Line, d.Pos.Col)
		}
	}
	toks, err := New("вывести `a // b /* c */ ${\"//\"}`").Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка лексера: %v", err)
	}
	for _, part := range toks[1].Parts {
		if part.Expr {
			if _, err := TemplatePartTokens(part); err != nil {
				t.Fatalf("\"//\" в строковом литерале внутри вставки - не комментарий: %v", err)
			}
		}
	}
}
