package main

import "testing"

func TestParseBuildArgsPlainPath(t *testing.T) {
	src, code, keepC, hasCode, err := parseBuildArgs([]string{"--keep-c", "прог.rech"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if src != "прог.rech" || code != "" || !keepC || hasCode {
		t.Fatalf("получено src=%q code=%q keepC=%v hasCode=%v", src, code, keepC, hasCode)
	}
}

func TestParseBuildArgsCodeFlag(t *testing.T) {
	for _, flag := range []string{"-c", "--code"} {
		src, code, _, hasCode, err := parseBuildArgs([]string{flag, "вывести 1"})
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if !hasCode || code != "вывести 1" || src != "" {
			t.Fatalf("флаг %q: получено src=%q code=%q hasCode=%v", flag, src, code, hasCode)
		}
	}
}

func TestParseBuildArgsCodeWithoutValueFails(t *testing.T) {
	_, _, _, _, err := parseBuildArgs([]string{"-c"})
	if err == nil {
		t.Fatal("ожидалась ошибка - после -c нет значения")
	}
}

func TestParseBuildArgsCodeAndPathConflictFails(t *testing.T) {
	_, _, _, _, err := parseBuildArgs([]string{"-c", "вывести 1", "прог.rech"})
	if err == nil {
		t.Fatal("ожидалась ошибка - -c и путь одновременно")
	}
}
