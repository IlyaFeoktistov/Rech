package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParsePlatformBlockTwoArms(t *testing.T) {
	prog, err := parseSource(t, `
#если ОС.Linux {
    функция сообщение(): Строка { вернуть "linux" }
} #иначе {
    функция сообщение(): Строка { вернуть "не-linux" }
}
функция главная { вывести сообщение() }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.PlatformBlocks) != 1 {
		t.Fatalf("ожидался один PlatformBlockDecl, получено: %d", len(prog.PlatformBlocks))
	}
	block := prog.PlatformBlocks[0]
	if len(block.Arms) != 2 {
		t.Fatalf("ожидалось 2 руки, получено: %d", len(block.Arms))
	}
	if block.Arms[0].Platform != "Linux" {
		t.Fatalf("первая рука: ожидалась платформа \"Linux\", получено %q", block.Arms[0].Platform)
	}
	if len(block.Arms[0].Functions) != 1 || block.Arms[0].Functions[0].Name != "сообщение" {
		t.Fatalf("первая рука: ожидалась функция \"сообщение\", получено %+v", block.Arms[0].Functions)
	}
	if block.Arms[1].Platform != "" {
		t.Fatalf("вторая рука (безусловный #иначе): ожидалась пустая платформа, получено %q", block.Arms[1].Platform)
	}
}

// TestParsePlatformBlockElseIfChain - "#иначе если" (цепочка, в отличие от
// обычного рантайм "если"/"иначе" - см. package doc ast.PlatformBlockDecl).
func TestParsePlatformBlockElseIfChain(t *testing.T) {
	prog, err := parseSource(t, `
#если ОС.Windows {
    внешняя функция CreateWindowExW(): Целое
} #иначе если ОС.MacOS {
    внешняя функция NSApplicationMain(): Целое
} #иначе {
    внешняя функция XOpenDisplay(): Целое
}
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	block := prog.PlatformBlocks[0]
	if len(block.Arms) != 3 {
		t.Fatalf("ожидалось 3 руки, получено: %d", len(block.Arms))
	}
	wantPlatforms := []string{"Windows", "MacOS", ""}
	for i, want := range wantPlatforms {
		if block.Arms[i].Platform != want {
			t.Fatalf("рука %d: ожидалась платформа %q, получено %q", i, want, block.Arms[i].Platform)
		}
	}
}

func TestParsePlatformBlockRejectsUnknownPlatform(t *testing.T) {
	_, err := parseSource(t, `
#если ОС.Solaris {
    функция сообщение(): Строка { вернуть "?" }
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "не существует") {
		t.Fatalf("ожидалась ошибка про несуществующую платформу, получено: %v", err)
	}
}

// TestParsePlatformBlockRejectsBareStatement - внутри "#если" осмысленны
// только объявления (функция/сущность/...), не обычные инструкции неявной
// "главная".
func TestParsePlatformBlockRejectsBareStatement(t *testing.T) {
	_, err := parseSource(t, `
#если ОС.Linux {
    вывести 1
}
функция главная { вывести 2 }
`)
	if err == nil || !strings.Contains(err.Error(), "не обычные инструкции") {
		t.Fatalf("ожидалась ошибка про недопустимую инструкцию внутри \"#если\", получено: %v", err)
	}
}

// TestParsePlatformBlockModuleVar - переменная модуля внутри "#если" -
// объявление выбранной ветки (ResolvePlatformBlocks), на своём месте по
// строке среди остальных переменных модуля.
func TestParsePlatformBlockModuleVar(t *testing.T) {
	prog, err := parseSource(t, "Целое первая = 1\n#если ОС.Linux {\n    Строка платформа = \"linux\"\n} #иначе {\n    Строка платформа = \"другая\"\n}\nЦелое последняя = 2\n\nфункция главная\n{\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ast.ResolvePlatformBlocks(prog, "Linux")
	if len(prog.Vars) != 3 || prog.Vars[1].Name != "платформа" {
		t.Fatalf("ожидались первая, платформа, последняя - получено %+v", prog.Vars)
	}
	if _, err := parseSource(t, "#если ОС.Linux {\n    Строка платформа\n}\n\nфункция главная\n{\n}\n"); err == nil {
		t.Fatal("ожидалась ошибка: у переменной модуля нужно значение")
	}
}
