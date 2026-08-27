package ast

// StampFile проставляет File (см. FunctionDecl.File/EntityDecl.File)
// каждому верхнеуровневому объявлению prog путём path - вызывается сразу
// после разбора ОДНОГО файла (internal/module.loader.load,
// internal/compiler.parseFileRaw), до того как несколько программ сольются
// в одну (module.Link/parseDir) - тогда путь ещё однозначно известен.
func StampFile(prog *Program, path string) {
	for _, fn := range prog.Functions {
		fn.File = path
	}
	for _, ent := range prog.Entities {
		ent.File = path
	}
}
