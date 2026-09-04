// Многофайловое обогащение однофайлового результата (см. package doc
// document.go) - открытый документ по-прежнему разбирается/анализируется
// САМ ПО СЕБЕ, как и раньше (d.tokens/d.astProg/d.irProg - живой буфер VS
// Code, единственный источник для позиционных запросов - hover/definition/
// completion/implementation по КУРСОРУ в этом файле), но если он (прямо
// или транзитивно) использует "использовать <имя> из "<путь>"", этот файл
// пытается ДОПОЛНИТЕЛЬНО разрешить граф импортов ТЕМ ЖЕ путём, что и
// настоящий "rech build <файл>" (internal/compiler.parseFile - module.Load/
// Program.Link) - чтобы "ЛоггерИнтерфейс"/"ФайловыйЛоггер" и т. п.,
// объявленные в ДРУГОМ файле, были видны hover/definition/completion/
// implementation этого документа, а не давали пустой результат только
// потому, что semantic.Analyze одного файла не может их разрешить (найдено
// при добавлении "go to implementation" - ЛЮБАЯ фича этого пакета молчала
// на файле с хотя бы одним "использовать", т. е. на любом файле типичного
// многофайлового проекта).
//
// ВАЖНОЕ УПРОЩЕНИЕ: module.Load читает path И ВСЕ файлы, на которые он
// ссылается, С ДИСКА (internal/module не умеет получать содержимое от LSP-
// клиента для НЕ открытых им файлов) - значит многофайловое обогащение
// видит последнее СОХРАНЁННОЕ состояние всего графа, а не то, что сейчас
// набрано, но ещё не сохранено, в других открытых вкладках. Позиционные
// запросы САМОГО этого документа (см. выше) это не задевает - они по
// живому буферу, как и раньше; задевает только то, какие ИМЕНА из других
// файлов видны разрешению (обновится, как только эти файлы будут
// сохранены). Известное, осознанное ограничение этого среза, не забытый
// баг.
package lsp

import (
	"os"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// linkResult - итог удачной многофайловой линковки (см. package doc).
type linkResult struct {
	ast          *ast.Program
	ir           *ir.Program
	entryImports []module.Import
}

// uriToPath - "file:///abs/путь.rech" -> "/abs/путь.rech"; "" для любого
// другого вида URI (в частности, тестовые "test://..." по всему пакету) -
// tryLink в этом случае просто не пытается ничего, см. package doc.
func uriToPath(uri string) string {
	const prefix = "file://"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return strings.TrimPrefix(uri, prefix)
}

// fileURI - обратное к uriToPath, для файлов, найденных на диске через
// граф импортов (module.Module.Path - уже filepath.Clean, см. package doc
// module.Module).
func fileURI(path string) string {
	return "file://" + path
}

// tryLink пытается разрешить path (и весь граф "использовать" от него) тем
// же путём, что internal/compiler.parseFile - см. package doc про то, с
// какого именно содержимого файлов. ok==false, path пуст/не .rech - линковка
// вообще не пыталась ничего (attemptErr тоже nil в этом случае - вызывающая
// сторона остаётся с однофайловым результатом молча, как и раньше). ok==
// false, attemptErr != nil - линковка ПЫТАЛАСЬ (файл не читается, ошибка
// импорта/цикла или семантики где-то в графе) - doc.update использует
// attemptErr как диагностику (см. diagnostics.go), она обычно куда
// информативнее однофайлового semErr (тот часто просто каскад "неизвестный
// тип" от самого факта отсутствия разрешения импорта).
func tryLink(path string) (res linkResult, ok bool, attemptErr error) {
	if path == "" || !strings.HasSuffix(path, ".rech") {
		return linkResult{}, false, nil
	}
	prog, err := module.Load(path, compiler.NewFrontend())
	if err != nil {
		return linkResult{}, false, err
	}
	merged, err := prog.Link()
	if err != nil {
		return linkResult{}, false, err
	}
	irProg, err := semantic.Analyze(merged)
	if err != nil {
		return linkResult{}, false, err
	}
	return linkResult{ast: merged, ir: irProg, entryImports: prog.Entry.Imports}, true, nil
}

// importedMergedName - под каким именем module.Program.Link() сохраняет
// объявление, стоящее за imp, в СЛИТОЙ программе - тем же правилом, что и
// её собственный internal/module.mangledName/buildNames (неэкспортированы,
// поэтому продублировано здесь по ЭКСПОРТИРОВАННЫМ данным - Import.Source.
// AST/Qualifier): интерфейс/перечисление/константа уровня модуля - под
// СВОИМ именем без изменений (Link не манглит эти три категории вообще -
// единая плоская область видимости на программу, см. package doc
// internal/module/link.go, buildNames), функция/сущность - с префиксом
// Qualifier исходного модуля.
func importedMergedName(imp module.Import) string {
	src := imp.Source.AST
	for _, iface := range src.Interfaces {
		if iface.Name == imp.Name {
			return imp.Name
		}
	}
	for _, e := range src.Enums {
		if e.Name == imp.Name {
			return imp.Name
		}
	}
	for _, c := range src.Consts {
		if c.Name == imp.Name {
			return imp.Name
		}
	}
	return imp.Source.Qualifier + "_" + imp.Name
}

// applyLink обогащает уже готовый однофайловый результат res (см. package
// doc doc/tryLink) - заменяет entities/funcs/astEntities/astFuncs/
// astInterfaces/irProg на версии из СЛИТОЙ программы (свои объявления там
// уже под тем же именем, что и в однофайловом проходе - Link не манглит
// entry-модуль, см. internal/module/link.go, mangledName) и добавляет по
// одному алиасу на каждый импорт ЭТОГО файла - имя, под которым импорт
// написан в его собственном "использовать", должно указывать на ту же
// запись, что и смангленное имя в слитой программе, иначе резолюция по
// "как написано в открытом документе" (chainBefore и весь resolve.go)
// просто не находила бы ничего для любого импортированного имени.
func (d *doc) applyLink(res linkResult) {
	d.irProg = res.ir
	d.entities = map[string]*ir.Entity{}
	for _, e := range res.ir.Entities {
		d.entities[e.Name] = e
	}
	d.funcs = map[string]*ir.Function{}
	for _, f := range res.ir.Functions {
		d.funcs[f.Name] = f
	}
	d.astEntities = map[string]*ast.EntityDecl{}
	for _, e := range res.ast.Entities {
		d.astEntities[e.Name] = e
	}
	d.astFuncs = map[string]*ast.FunctionDecl{}
	for _, f := range res.ast.Functions {
		d.astFuncs[f.Name] = f
	}
	d.astInterfaces = map[string]*ast.InterfaceDecl{}
	for _, iface := range res.ast.Interfaces {
		d.astInterfaces[iface.Name] = iface
	}
	d.displayName = map[string]string{}
	d.localNames = map[string]bool{}
	for _, e := range res.ast.Entities {
		if d.isCurrentFile(e.File) {
			d.localNames[e.Name] = true
		}
	}
	for _, f := range res.ast.Functions {
		if d.isCurrentFile(f.File) {
			d.localNames[f.Name] = true
		}
	}
	for _, imp := range res.entryImports {
		mangled := importedMergedName(imp)
		d.localNames[imp.Alias] = true
		if mangled != imp.Alias {
			d.displayName[mangled] = imp.Alias
		}
		if e, ok := d.entities[mangled]; ok {
			d.entities[imp.Alias] = e
		}
		if f, ok := d.funcs[mangled]; ok {
			d.funcs[imp.Alias] = f
		}
		if e, ok := d.astEntities[mangled]; ok {
			d.astEntities[imp.Alias] = e
		}
		if f, ok := d.astFuncs[mangled]; ok {
			d.astFuncs[imp.Alias] = f
		}
		if iface, ok := d.astInterfaces[mangled]; ok {
			d.astInterfaces[imp.Alias] = iface
		}
	}
}

// isCurrentFile - file (ast.EntityDecl.File/ast.FunctionDecl.File, см.
// ast.StampFile) обозначает ТЕКУЩИЙ открытый документ - под ОДНИМ из двух
// видов, в зависимости от того, откуда взят file: d.uri у объявлений из
// однофайлового прохода (ast.StampFile(astProg, d.uri) в update()), d.path
// (настоящий путь на диске) у объявлений ИЗ СЛИТОЙ программы, где entry-
// модуль стемпается module.Load СВОИМ путём, не d.uri - см. tryLink.
func (d *doc) isCurrentFile(file string) bool {
	return file == d.uri || (d.path != "" && file == d.path)
}

// foreignTokens - токены ДРУГОГО файла графа импортов, прочитанного и
// разобранного с диска по требованию (declPosInFile) - копится в
// d.foreignTokenCache на время жизни текущего "последнего удачного"
// разбора документа (см. package doc document.go). false - файл не
// читается (переименован/удалён с последней линковки) или не лексится.
func (d *doc) foreignTokens(file string) ([]lexer.Token, bool) {
	if toks, ok := d.foreignTokenCache[file]; ok {
		return toks, true
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, false
	}
	toks, err := lexer.New(string(src)).Tokenize()
	if err != nil {
		return nil, false
	}
	d.foreignTokenCache[file] = toks
	return toks, true
}
