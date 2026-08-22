package module

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// mangledName - имя, под которым объявление name из модуля m в итоге
// попадает в слинкованную программу: без изменений у entry (единственный
// модуль, чьи имена видны программе снаружи - как единица компиляции,
// раньше это тоже был всегда единственный файл), с префиксом Qualifier у
// всех остальных - модули загружаются независимо друг от друга и могут
// использовать одно и то же имя для разных вещей (сама идея пространств
// имён), поэтому при слиянии в одну плоскую internal/ast.Program (её всё
// ещё разбирает единственный проход cbackend/semantic, см. package doc)
// имена нужно развести, иначе они столкнутся так же, как если бы это был
// один файл.
func mangledName(m, entry *Module, name string) string {
	if m == entry {
		return name
	}
	return m.Qualifier + "_" + name
}

// buildNames строит карту "имя, как написано в файле m" -> "итоговое имя
// после линковки" - для собственных объявлений m (включая производные типы
// состояний - localTypeNames, а не только то, что можно импортировать) и
// для того, что m импортировал (Import.Alias - как оно написано в теле m -
// -> mangledName настоящего Import.Name в модуле-источнике; переименование
// при импорте, "как", - именно поэтому ключ здесь Alias, а не Name).
func buildNames(m, entry *Module) map[string]string {
	names := map[string]string{}
	for name := range localTypeNames(m.AST) {
		names[name] = mangledName(m, entry, name)
	}
	for _, imp := range m.Imports {
		names[imp.Alias] = mangledName(imp.Source, entry, imp.Name)
	}
	return names
}

// Link проверяет видимость (каждая ссылка на функцию/тип в теле функции,
// метода или в сигнатуре сущности должна быть либо объявлена в том же
// файле, либо явно импортирована - иначе ошибка компиляции, а не то, что
// уже сейчас происходит в режиме каталога, где виден вообще весь каталог)
// и возвращает одну объединённую internal/ast.Program - тем же интерфейсом,
// что и раньше принимают cbackend.Generate/semantic.Analyze, поэтому им не
// нужно знать про модули: к моменту, когда они получают Program, все имена
// уже глобально уникальны (переименование по mangledName) и синтаксически
// корректны в изначальном, невозможном для коллизии виде.
func (p *Program) Link() (*ast.Program, error) {
	for _, m := range p.Modules {
		names := buildNames(m, p.Entry)
		r := &rewriter{names: names}
		for _, fn := range m.AST.Functions {
			if err := r.rewriteParams(fn.Params); err != nil {
				return nil, fmt.Errorf("%s: функция %q: %w", m.Path, fn.Name, err)
			}
			resolved, err := r.rewriteReturnType(fn.ReturnType)
			if err != nil {
				return nil, fmt.Errorf("%s: функция %q: тип результата: %w", m.Path, fn.Name, err)
			}
			fn.ReturnType = resolved
			if err := r.rewriteStmts(fn.Body); err != nil {
				return nil, fmt.Errorf("%s: функция %q: %w", m.Path, fn.Name, err)
			}
		}
		for _, ent := range m.AST.Entities {
			if err := r.rewriteFields(ent.Fields); err != nil {
				return nil, fmt.Errorf("%s: сущность %q: %w", m.Path, ent.Name, err)
			}
			for _, meth := range ent.Methods {
				if err := r.rewriteParams(meth.Params); err != nil {
					return nil, fmt.Errorf("%s: сущность %q: метод %q: %w", m.Path, ent.Name, meth.Name, err)
				}
				resolved, err := r.rewriteReturnType(meth.ReturnType)
				if err != nil {
					return nil, fmt.Errorf("%s: сущность %q: метод %q: тип результата: %w", m.Path, ent.Name, meth.Name, err)
				}
				meth.ReturnType = resolved
				if err := r.rewriteStmts(meth.Body); err != nil {
					return nil, fmt.Errorf("%s: сущность %q: метод %q: %w", m.Path, ent.Name, meth.Name, err)
				}
			}
		}

		// Переименование собственных объявлений m - после переписывания тел
		// выше (rewriter резолвит по ИСХОДНЫМ, ещё не переименованным именам
		// из names), не до: порядок между этим блоком и предыдущим на самом
		// деле не важен (разные поля разных узлов), но так нагляднее -
		// сначала решить, что куда ссылается, потом переименовать то, на что
		// ссылаются.
		if m != p.Entry {
			for _, fn := range m.AST.Functions {
				fn.Name = mangledName(m, p.Entry, fn.Name)
			}
			for _, ent := range m.AST.Entities {
				ent.Name = mangledName(m, p.Entry, ent.Name)
				for i := range ent.States {
					ent.States[i].TypeName = mangledName(m, p.Entry, ent.States[i].TypeName)
				}
			}
		}
	}

	merged := &ast.Program{}
	for _, m := range p.Modules {
		merged.Functions = append(merged.Functions, m.AST.Functions...)
		merged.Entities = append(merged.Entities, m.AST.Entities...)
	}
	return merged, nil
}
