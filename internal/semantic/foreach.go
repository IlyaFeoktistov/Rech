package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// forEachKindByTemplate - вид цикла "для x в ..." по шаблону встроенной
// generic-коллекции (parseGenericTypeName).
var forEachKindByTemplate = map[string]ir.ForEachKind{
	"Список":  ir.ForEachList,
	"Стопка":  ir.ForEachStack,
	"Очередь": ir.ForEachQueue,
	"Словарь": ir.ForEachDict,
}

// analyzeForEachStmt - "для [<ключ>,] <имя> в <коллекция>". Файл -
// потоковое построчное чтение (не индексированный доступ - файл не
// известной заранее длины), ElemType всегда Строка, формы с ключом нет.
// Встроенные коллекции - элемент ElemType; ключ - KeyType у Словаря,
// порядковый номер обхода (Целое) у Список/Стопка/Очередь.
func analyzeForEachStmt(sc *ctx, s *ast.ForEachStmt) (ir.Stmt, error) {
	collection, err := analyzeExpr(sc, s.Collection)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(collection)
	if typ.Kind == ir.KindFile {
		if s.KeyVar != "" {
			return nil, errAtLine(s.Line, CodeStmtCheck, "цикл \"для %s, %s в ...\" по Файлу не поддерживается - только \"для %s в ...\" (построчно)", s.KeyVar, s.Var, s.Var)
		}
		body, err := analyzeLoopBody(sc, s.Body, s.Line, loopVar{s.Var, ir.TypeString})
		if err != nil {
			return nil, err
		}
		return &ir.ForEachStmt{Kind: ir.ForEachFile, Var: s.Var, Collection: collection, ElemType: ir.TypeString, Body: body, Line: s.Line}, nil
	}
	kind, ok := ir.ForEachList, false
	if typ.Kind == ir.KindEntity && isBuiltinCollectionType(sc.entities, typ) {
		if template, _, parsed := parseGenericTypeName(typ.Entity); parsed {
			kind, ok = forEachKindByTemplate[template]
		}
	}
	if !ok {
		return nil, errAtLine(s.Line, CodeStmtCheck, "цикл \"для %s в ...\" ожидает переменную-Список/Стопку/Очередь/Словарь или Файл, получен тип %s", s.Var, typ)
	}
	sig := sc.entities[typ.Entity]
	keyType := ir.TypeInt
	if kind == ir.ForEachDict {
		keyType = sig.KeyType
	}
	vars := []loopVar{{s.Var, sig.ElemType}}
	if s.KeyVar != "" {
		vars = []loopVar{{s.KeyVar, keyType}, {s.Var, sig.ElemType}}
	}
	body, err := analyzeLoopBody(sc, s.Body, s.Line, vars...)
	if err != nil {
		return nil, err
	}
	return &ir.ForEachStmt{
		Kind:       kind,
		KeyVar:     s.KeyVar,
		KeyType:    keyType,
		Var:        s.Var,
		Collection: collection,
		ElemType:   sig.ElemType,
		Body:       body,
		Line:       s.Line,
	}, nil
}

// collectionListType - "Список<T>" результата "ключи()"/"значения()" у
// Стопка/Очередь/Словарь (в порядке обхода "для x в ..."). Сама
// инстанциация регистрируется лениво, при первом вызове такого метода
// (registerCollectionResultType) - иначе каждая Стопка/Очередь/Словарь в
// программе тянула бы неиспользуемый typedef Список<T>.
func collectionListType(elem ir.Type) ir.Type {
	return ir.EntityType("Список<" + elem.String() + ">")
}

// registerCollectionResultType - вызывается analyzeMethodCallExpr для
// метода встроенной коллекции recv: если тип результата - ещё не
// зарегистрированная generic-инстанциация (см. collectionListType),
// регистрирует её (resolveTypeName идемпотентен).
func registerCollectionResultType(sc *ctx, recv ir.Type, sig funcSig) error {
	if !sig.HasReturn || sig.ReturnType.Kind != ir.KindEntity || !isBuiltinCollectionType(sc.entities, recv) {
		return nil
	}
	if _, ok := sc.entities[sig.ReturnType.Entity]; ok {
		return nil
	}
	_, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, sig.ReturnType.Entity)
	return err
}
