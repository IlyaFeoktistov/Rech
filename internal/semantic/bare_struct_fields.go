package semantic

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// Структура, объявленная без значения ("Пара п"), заполняется по полям:
// "п.a = 1" - запись поля (не чтение п), чтение поля "п.a" - только после
// его записи, чтение всей "п" (передача, вызов метода, присваивание) -
// когда записаны все поля без значения по умолчанию. Поле с модификатором
// "приватный" снаружи записать нельзя (checkFieldVisibility) - такую
// структуру создают конструктором. Сущность (ссылочный тип) по полям не
// заполняется: пока ей не присвоен объект, полей у неё нет.

// fieldAssignedState - факт "поле записано" в established переменной.
const fieldAssignedState = "\x00поле:"

// bareStruct - e - имя структуры, объявленной без значения и ещё не
// присвоенной целиком: её имя, тип и сигнатура.
func bareStruct(sc *ctx, e ast.Expr) (*ast.Identifier, ir.Type, entitySig, bool) {
	id, ok := e.(*ast.Identifier)
	if !ok || !sc.readBeforeAssign(id.Name) {
		return nil, ir.Type{}, entitySig{}, false
	}
	typ, _ := sc.lookup(id.Name)
	sig, ok := sc.entities[typ.Entity]
	if typ.Kind != ir.KindEntity || !ok {
		return nil, ir.Type{}, entitySig{}, false
	}
	return id, typ, sig, true
}

// fieldAssigned - поле field переменной name уже записано на каждом пути.
func (c *ctx) fieldAssigned(name, field string) bool {
	return c.getEstablished(name)[fieldAssignedState+field]
}

// markFieldAssigned - поле field переменной name записано; если записаны
// все поля без значения по умолчанию - присвоена и вся переменная.
func (c *ctx) markFieldAssigned(name, field string, sig entitySig) {
	states := cloneStates(c.getEstablished(name))
	states[fieldAssignedState+field] = true
	c.setEstablished(name, states)
	if len(c.missingFields(name, sig)) == 0 {
		c.setEstablished(name, c.withAssigned(name, states))
	}
}

// missingFields - поля name без значения по умолчанию, ещё не записанные.
func (c *ctx) missingFields(name string, sig entitySig) []string {
	var out []string
	for i, f := range sig.FieldNames {
		// Поле-массив заполнено нулями (array.go) - уже записано.
		if sig.FieldDefaultExprs[i] == nil && sig.FieldTypes[i].Kind != ir.KindArray && !c.fieldAssigned(name, f) {
			out = append(out, f)
		}
	}
	return out
}

// errReadBareStruct - чтение всей структуры name, у которой записаны не все
// поля; приватное среди них - подсказка про конструктор.
func errReadBareStruct(sc *ctx, name string, typ ir.Type, sig entitySig, line int) error {
	missing := sc.missingFields(name, sig)
	d := errAtLine(line, CodeStmtCheck, "структура %q читается до того, как заданы все её поля - не заданы: %s", name, strings.Join(missing, ", "))
	for _, f := range missing {
		if idx := sig.fieldIndex(f); idx != -1 && sig.FieldVisibility[idx] != ast.Public {
			return d.WithHint("поле %q приватное - создайте структуру конструктором: %s = %s %s(...)", f, name, newWordFor(typ.Entity), typ.Entity)
		}
	}
	return d.WithHint("задайте их до этого места: %s.%s = ...", name, missing[0])
}

// newWordFor - "новый"/"новая"/"новое"/"новые", согласованное с entity.
func newWordFor(entity string) string {
	gender, number, _ := morph.CompoundNounAgreement(entity)
	return morph.NewKeywordFor(gender, number)
}
