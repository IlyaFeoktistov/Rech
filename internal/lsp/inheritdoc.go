// "@наследовать" в doc-комментарии метода (аналог JSDoc "@inheritDoc",
// свой русский тег - тем же принципом, что и "@устарело",
// deprecated.go) - метод, ПЕРЕОПРЕДЕЛИВШИЙ метод базовой сущности (своё
// тело, поэтому попал в Entity.Methods - см. её package doc,
// internal/ir/decl.go, Entity.BaseName), но не написавший СВОЙ
// doc-комментарий (только тег) - hover показывает doc БАЗОВОГО метода
// вместо пустого/тега как есть. Метод без своего doc-комментария
// наследует его так же, без тега. После цепочки предков ищется метод
// реализуемого интерфейса ("реализует", ast.InterfaceMethod.Doc).
package lsp

import (
	"regexp"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// inheritRe - "@наследовать" где угодно в doc - директива, не текст для
// показа как есть (в отличие от "@устарело", у которой после тега может
// идти сообщение) - у "@наследовать" ничего после тега не ожидается и не
// используется, сам факт присутствия - и есть весь сигнал.
var inheritRe = regexp.MustCompile(`(?i)@наследовать`)

// hasInheritTag - см. package doc.
func hasInheritTag(doc string) bool {
	return inheritRe.MatchString(doc)
}

// resolvedMethodDoc - doc, который стоит показать в hover для метода name
// сущности ent, чей СОБСТВЕННЫЙ doc-комментарий - ownDoc: если в нём нет
// "@наследовать" - ownDoc без изменений; если есть - doc БЛИЖАЙШЕГО
// предка по цепочке BaseName, где name переопределён/впервые объявлен И
// несёт непустой doc (см. inheritedMethodDoc) - не найдено ни у одного
// предка (тег написан зря, либо сущность-предок не в этом же документе) -
// возвращает ownDoc как есть (тег останется виден буквально - честнее,
// чем молча показать пустоту).
func (d *doc) resolvedMethodDoc(ent *ir.Entity, name, ownDoc string) string {
	if ownDoc != "" && !hasInheritTag(ownDoc) {
		return ownDoc
	}
	if inherited := d.inheritedMethodDoc(ent, name); inherited != "" {
		return inherited
	}
	return ownDoc
}

// inheritedMethodDoc ищет метод name у сущности БАЗОВОЙ по отношению к ent
// (BaseName, дальше вверх по цепочке, ближайший предок - первым) - первый
// непустой Doc. "" - ни у одного предка нет непустого Doc метода name
// (либо доступной по имени сущности вовсе нет в этом документе - см.
// d.entities, multifile.go, включает и сущности из других файлов).
func (d *doc) inheritedMethodDoc(ent *ir.Entity, name string) string {
	chain := []*ir.Entity{ent}
	baseName := ent.BaseName
	for baseName != "" {
		base, ok := d.entities[baseName]
		if !ok {
			break
		}
		for _, m := range base.Methods {
			if m.Name == name && m.Doc != "" {
				return m.Doc
			}
		}
		chain = append(chain, base)
		baseName = base.BaseName
	}
	for _, e := range chain {
		for _, iface := range e.Interfaces {
			if m, _, ok := d.findInterfaceMethod(iface, name, map[string]bool{}); ok && m.Doc != "" {
				return m.Doc
			}
		}
	}
	return ""
}
