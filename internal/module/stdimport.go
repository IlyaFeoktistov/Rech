package module

import (
	"errors"
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/stdlib"
)

// Импорт встроенных имён из стандартных модулей (internal/stdlib). Без
// импорта встроенное имя не из ядра недоступно и свободно: файл может
// объявить своё с таким же именем (mangledName разводит его со встроенным).

// addStdImports - "использовать ... из стандарт/<модуль>": каждое имя
// обязано принадлежать модулю, псевдоним - быть свободным в файле.
func (m *Module) addStdImports(use *ast.UseDecl, declared map[string]bool) error {
	module, _ := stdlib.SplitPath(use.Path)
	if !stdlib.IsModule(module) {
		return fmt.Errorf("строка %d: стандартного модуля %q нет (есть: %v)", use.Line, module, stdlib.ModuleNames())
	}
	for _, un := range use.Names {
		if !stdlib.Exports(module, un.Name) {
			if other, ok := stdlib.ModuleOf(un.Name); ok {
				return fmt.Errorf("строка %d: %q - не из %s, а из %s%s", un.Line, un.Name, use.Path, stdlib.Prefix, other)
			}
			return fmt.Errorf("строка %d: в %s нет %q", un.Line, use.Path, un.Name)
		}
		if declared[un.Alias] {
			return fmt.Errorf("строка %d: имя %q уже объявлено в этом файле - используйте \"как\" для переименования при импорте", un.Line, un.Alias)
		}
		if err := m.checkAliasFree(un); err != nil {
			return err
		}
		m.StdImports = append(m.StdImports, StdImport{Alias: un.Alias, Name: un.Name})
	}
	return nil
}

// checkAliasFree - псевдоним un ещё не занят другим импортом файла.
func (m *Module) checkAliasFree(un ast.UseName) error {
	for _, existing := range m.Imports {
		if existing.Alias == un.Alias {
			return fmt.Errorf("строка %d: имя %q уже импортировано (из %q) - используйте \"как\" для переименования одного из импортов",
				un.Line, un.Alias, existing.Source.Path)
		}
	}
	for _, existing := range m.StdImports {
		if existing.Alias == un.Alias {
			return fmt.Errorf("строка %d: имя %q уже импортировано (из %s) - используйте \"как\" для переименования одного из импортов",
				un.Line, un.Alias, stdlib.Prefix+mustModule(existing.Name))
		}
	}
	return nil
}

// isBuiltinName - name встроено в язык (функция, тип или сущность).
func isBuiltinName(name string) bool {
	return builtinTypeBases[name] || builtins.IsReservedName(name)
}

// errNotImported - встроенное имя из стандартного модуля без импорта.
type errNotImported struct{ name string }

func (e errNotImported) Error() string {
	return fmt.Sprintf("%q не объявлено в этом файле и не импортировано - встроенное имя из %s: добавьте \"%s\"",
		e.name, stdlib.Prefix+mustModule(e.name), stdlib.ImportLine(e.name))
}

func mustModule(name string) string {
	m, _ := stdlib.ModuleOf(name)
	return m
}

// IsNotImportedBuiltin - err сообщает о встроенном имени без импорта
// (internal/lsp - быстрое исправление); name - само имя.
func IsNotImportedBuiltin(err error) (name string, ok bool) {
	var e errNotImported
	if errors.As(err, &e) {
		return e.name, true
	}
	return "", false
}
