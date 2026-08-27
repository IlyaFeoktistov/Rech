package lsp

import "testing"

func TestDefinitionOnFunctionCallJumpsToDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вывести сложить(1, 2)")
	col := runeCol(t, "    вывести сложить(1, 2)", "сложить")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение функции \"сложить\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "функция сложить")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnEntityConstructionJumpsToDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "новая Точка(1, 2)")
	col := runeCol(t, "    переменная т = новая Точка(1, 2)", "Точка")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение сущности \"Точка\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "сущность Точка")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnFieldAccessJumpsToFieldDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вывести т.x")
	col := runeCol(t, "    вывести т.x", "x")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение поля \"x\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "x: Целое")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnMethodCallJumpsToMethodDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "т.длина()")
	col := runeCol(t, "    вывести т.длина()", "длина")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение метода \"длина\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "длина(): Целое")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnConstantAccessJumpsToConstDecl(t *testing.T) {
	d := newDoc("test://b.rech", constHoverSrc)
	line := lineOf(t, constHoverSrc, "вывести т.ПРЕДЕЛ")
	col := runeCol(t, "    вывести т.ПРЕДЕЛ", "ПРЕДЕЛ")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение константы \"ПРЕДЕЛ\"")
	}
	wantLine := lineOf(t, constHoverSrc, "константа ПРЕДЕЛ")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}
