// Глагольные семьи (шаг 6.3): тег "противоположность:другойМетод" у
// объявления метода сущности связывает его с другим методом той же
// сущности как обратный переход состояния - см. package doc
// funcSig.RevokesStates, analyzeMethodCallExpr (internal/semantic/expr.go).
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// resolveOpposesTag разбирает тег m.Tag (если он есть) и вычисляет набор
// состояний, которые снимаются с получателя при вызове m - см. package doc
// funcSig.RevokesStates. siblings - все методы той же сущности (для поиска
// названного в теге), states - уже собранный sig.States сущности (полная
// форма -> краткая). nil, nil - у метода нет такого тега.
// overloadedNames - имена методов той же сущности, перегруженных (2+
// объявления) - target ПО ИМЕНИ среди siblings ниже не различает разные
// перегрузки одного имени (siblings - плоский список всех ast.MethodDecl
// сущности, включая все объявления группы), поэтому перегруженный target
// отклоняется явной ошибкой, а не резолвится в первое попавшееся
// объявление произвольно.
func resolveOpposesTag(m *ast.MethodDecl, siblings []*ast.MethodDecl, states map[string]string, overloadedNames map[string]bool) (map[string]bool, error) {
	target := structtag.Parse(m.Tag).Get("противоположность")
	if target == "" {
		return nil, nil
	}
	if target == m.Name {
		return nil, errNoPos(CodeStateVerb, "метод %q: нельзя объявить противоположностью самому себе", m.Name)
	}
	if overloadedNames[target] {
		return nil, errNoPos(CodeStateVerb, "метод %q: противоположность %q - метод с таким именем перегружен (несколько объявлений), неоднозначно, какую из перегрузок отменяет тег", m.Name, target)
	}
	var other *ast.MethodDecl
	for _, s := range siblings {
		if s.Name == target {
			other = s
			break
		}
	}
	if other == nil {
		return nil, errNoPos(CodeNameResolution, "метод %q: противоположность %q - нет метода с таким именем у этой же сущности", m.Name, target)
	}
	if other.IsAbstract {
		return nil, errNoPos(CodeStateVerb, "метод %q: противоположность %q - у абстрактного метода нет тела, нечего отменять", m.Name, target)
	}
	revoke := map[string]bool{}
	collectBecomeStates(other.Body, states, revoke)
	if len(revoke) == 0 {
		return nil, errNoPos(CodeStateVerb, "метод %q: противоположность %q не устанавливает ни одного состояния через \"становится\" - нечего отменять", m.Name, target)
	}
	return revoke, nil
}

// collectBecomeStates рекурсивно обходит body в поисках "этот становится
// <форма>" и добавляет в out краткую форму каждого найденного состояния
// (резолвится через states, sig.States - полная форма -> краткая). Не
// flow-sensitive (в отличие от established/GuaranteedThisStates) - находит
// переход внутри ЛЮБОЙ ветки (если/иначе, цикл, попытаться/поймать), не
// только гарантированный на всех путях выполнения: для отмены состояния это
// безопасное над-приближение - лишнее снятие никогда не разрешает
// использовать объект, не заслуживший состояние, только требует установить
// его заново.
func collectBecomeStates(body []ast.Stmt, states map[string]string, out map[string]bool) {
	for _, st := range body {
		switch s := st.(type) {
		case *ast.BecomeStmt:
			if _, ok := s.Receiver.(*ast.ThisExpr); ok {
				if base, ok := states[s.State]; ok {
					out[base] = true
				}
			}
		case *ast.IfStmt:
			collectBecomeStates(s.Then, states, out)
			collectBecomeStates(s.Else, states, out)
		case *ast.WhileStmt:
			collectBecomeStates(s.Body, states, out)
		case *ast.ForStmt:
			collectBecomeStates(s.Body, states, out)
		case *ast.ForEachStmt:
			collectBecomeStates(s.Body, states, out)
		case *ast.CStyleForStmt:
			collectBecomeStates(s.Body, states, out)
		case *ast.TryStmt:
			collectBecomeStates(s.Body, states, out)
			collectBecomeStates(s.CatchBody, states, out)
		case *ast.GerundStmt:
			collectBecomeStates(s.Body, states, out)
		case *ast.SwitchStmt:
			for _, c := range s.Cases {
				collectBecomeStates(c.Body, states, out)
			}
			collectBecomeStates(s.Default, states, out)
		}
	}
}
