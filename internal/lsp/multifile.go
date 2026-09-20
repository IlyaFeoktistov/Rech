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
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/frontend"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/project"
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
// tryLink в этом случае просто не пытается ничего, см. package doc. VS Code
// всегда шлёт URI с process-encoded (%D0%BF...) не-ASCII сегментами пути
// (кириллица - обычное имя файла/каталога Rech-проекта) - url.Parse их
// декодирует сам (.Path уже настоящая строка с кириллицей как есть), в
// отличие от naive strings.TrimPrefix (буквально оставил бы "%D0%BF..." в
// пути, os.ReadFile/module.Load искали бы файл с ТАКИМ именем и не находили
// вовсе - живая проверка на examples/проекты/http_сервер поймала именно
// это).
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return uriPathToOSPath(u.Path)
}

// uriPathToOSPath - обратное к osPathToURIPath ниже: "/C:/Users/..." (буква
// диска Windows, экранированная лишним ведущим "/" по RFC 8089) -> "C:/...",
// "/tmp/..." (обычный Unix-путь) остаётся как есть - отличить одно от
// другого можно только по виду самого пути (буква диска + ":" сразу после
// ведущего "/"), в самом URI разметки об ОС нет. filepath.FromSlash в конце
// - на Windows "/" внутри пути (в т. ч. дальше буквы диска) не то же самое,
// что "\", module.Load/os.ReadFile ждут родной разделитель.
func uriPathToOSPath(p string) string {
	if len(p) >= 4 && p[0] == '/' && isDriveLetter(p[1]) && p[2] == ':' && p[3] == '/' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func isDriveLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// fileURI - обратное к uriToPath, для файлов, найденных на диске через
// граф импортов (module.Module.Path - уже filepath.Clean, см. package doc
// module.Module) - url.URL.String() сам percent-encode'ит не-ASCII сегменты
// path обратно (симметрично uriToPath), не строит их конкатенацией строк.
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: osPathToURIPath(path)}).String()
}

// osPathToURIPath - "C:\Users\..." (Windows) -> "/C:/Users/..." (RFC 8089 -
// буква диска экранируется ЛИШНИМ ведущим "/", иначе "file://C:\Users\..."
// у url.URL.String() выглядит как "file://<host>:<port>..." - "C" читается
// как host, "\Users\..." - как невалидный порт); "/tmp/..." (Unix) уже
// начинается с "/" - остаётся как есть, лишний "/" не добавляется.
// filepath.ToSlash - url.URL.Path ожидает "/" как разделитель сегментов
// пути, не "\".
func osPathToURIPath(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// tryLink пытается разрешить path (и весь граф "использовать" от него) тем
// же путём, что internal/compiler.parseFile - см. package doc про то, с
// какого именно содержимого файлов. ok==false, path пуст/без расширения
// исходника (module.HasSourceExtension) - линковка вообще не пыталась
// ничего (attemptErr тоже nil в этом случае - вызывающая сторона остаётся с
// однофайловым результатом молча, как и раньше). ok==
// false, attemptErr != nil - линковка ПЫТАЛАСЬ (файл не читается, ошибка
// импорта/цикла или семантики где-то в графе) - doc.update использует
// attemptErr как диагностику (см. diagnostics.go), она обычно куда
// информативнее однофайлового semErr (тот часто просто каскад "неизвестный
// тип" от самого факта отсутствия разрешения импорта).
func tryLink(path string) (res linkResult, ok bool, attemptErr error) {
	if path == "" || !module.HasSourceExtension(path) {
		return linkResult{}, false, nil
	}
	// root - каталог главного файла проекта, если path лежит под каким-то
	// .rechproj (обычный случай для открытого документа многофайлового
	// проекта - он редко и есть сам главный файл) - без этого
	// "использовать ... из "@путь"" внутри path резолвился бы от каталога
	// САМОГО path, а не от корня программы, и не находил бы файлы в
	// соседних подкаталогах (см. internal/project.RootFor,
	// internal/module.LoadWithRoot). "" (path вне какого-либо .rechproj) -
	// module.LoadWithRoot ведёт себя как раньше (module.Load).
	root, err := project.RootFor(path)
	if err != nil {
		return linkResult{}, false, err
	}
	prog, err := module.LoadWithRoot(path, frontend.NewFrontend(), nil, nil, root)
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
	d.astEnums = map[string]*ast.EnumDecl{}
	for _, e := range res.ast.Enums {
		d.astEnums[e.Name] = e
	}
	d.astConsts = map[string]*ast.ConstDecl{}
	for _, c := range res.ast.Consts {
		d.astConsts[c.Name] = c
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
	// ast.InterfaceDecl не несёт File вовсе (в отличие от Entity/Function
	// выше) - "своя" проверка идёт через d.astProg (ВСЕГДА разбор именно
	// этого буфера, см. package doc document.go), не res.ast (объединённое
	// дерево всего графа импортов) - тот же приём, что уже использует
	// codelens.go для "собственных объявлений документа".
	for _, iface := range d.astProg.Interfaces {
		d.localNames[iface.Name] = true
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

// fileImportsTarget - true, если candidate (прямо или транзитивно, через
// цепочку "использовать") реально ссылается на target - candidate
// разрешается СВОИМ собственным графом импортов (module.LoadWithRoot от
// candidate КАК ГЛАВНОГО файла, тем же способом, что tryLink разрешает
// открытый документ), target ищется среди получившихся Module.Path.
//
// Нужно crossFileRefs (references.go), чтобы не путать СЛУЧАЙНОЕ текстовое
// совпадение имени с настоящей ссылкой: два файла (в том числе из ДВУХ
// РАЗНЫХ проектов, соседствующих на диске - например, два открытых рядом
// .rechproj под общим родительским каталогом) могут объявлять сущность/
// функцию с одинаковым именем, ничего не зная друг о друге - раньше
// "N ссылок" считало по всему поддереву каталога (workspace_scan.go,
// scanRoot) ЛЮБОЕ текстовое совпадение, не проверяя, способен ли candidate
// вообще ВИДЕТЬ объявление из target (репортнуто пользователем). false и
// при любой ошибке разбора candidate (её граф не строится - не то, что
// нужно ЭТОЙ проверке, показать её отдельной диагностикой не для чего) -
// безопаснее занизить число ссылок, чем посчитать случайное совпадение.
func fileImportsTarget(candidate, target string) bool {
	root, err := project.RootFor(candidate)
	if err != nil {
		return false
	}
	prog, err := module.LoadWithRoot(candidate, frontend.NewFrontend(), nil, nil, root)
	if err != nil {
		return false
	}
	target = filepath.Clean(target)
	for _, mod := range prog.Modules {
		if mod.Path == target {
			return true
		}
	}
	return false
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
