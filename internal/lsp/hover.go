package lsp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// hover строит текст всплывающей подсказки для позиции (line, col, оба
// 1-индексные) - "", false, если под курсором нечего показать (пробел,
// ключевое слово, нераспознанный идентификатор).
func (d *doc) hover(line, col int) (string, bool) {
	text, ok := d.hoverText(line, col)
	return ir.DisplayTypes(text), ok
}

// hoverText - текст hover до перевода канонических имён типов в запись
// исходника (ir.DisplayTypes).
func (d *doc) hoverText(line, col int) (string, bool) {
	if d.irProg == nil {
		return "", false
	}
	i := d.tokenIndexAt(line, col)
	if i < 0 {
		return "", false
	}
	tok := d.tokens[i]
	if doc, ok := keywordDocs[tok.Type]; ok {
		return doc, true
	}
	if tok.Type != lexer.IDENT && tok.Type != lexer.KwThis {
		return "", false
	}
	chain := d.chainBefore(i)
	if len(chain) == 0 {
		return "", false
	}
	name := chain[len(chain)-1]

	if len(chain) == 1 {
		return d.hoverBareName(name, line)
	}

	container, ok := d.resolveChain(chain[:len(chain)-1], line)
	if !ok {
		// resolveChain не знает о перечислениях (chain[0] - не локальная
		// переменная/поле/сущность) - "Имя.Член" только ДВУХзвенное (в
		// самом языке нет вложенных перечислений), проверяем именно этот
		// случай отдельным путём (enumhover.go), прежде чем сдаться.
		if len(chain) == 2 {
			if block, ok := d.hoverEnumMember(chain[0], name); ok {
				return block, true
			}
		}
		return "", false
	}
	if container.Kind != ir.KindEntity {
		return "", false
	}
	ent := d.entities[container.Entity]
	if ent == nil {
		// Не пользовательская сущность (и не одна из системных, лениво
		// зарегистрированных при семантическом анализе) - последний шанс,
		// встроенная generic-коллекция (Список<T> и т. п., см. package doc
		// genericcollection.go) - у неё нет собственного *ir.Entity вовсе.
		return hoverGenericCollectionMember(container.Entity, name)
	}
	return d.hoverMember(ent, name)
}

// hoverBareName - курсор на самостоятельном имени (не после точки):
// локальная переменная/параметр, "этот", функция или сущность верхнего
// уровня, в этом порядке (первое совпадение побеждает).
func (d *doc) hoverBareName(name string, line int) (string, bool) {
	scope := d.scopeAt(line)
	if name == "этот" && scope != nil && scope.entityName != "" {
		return codeBlock(fmt.Sprintf("этот: %s", scope.entityName)), true
	}
	if scope != nil {
		// Параметр - от обычной локальной переменной, чтобы hover сразу
		// говорил, что это (пользователь попросил явно - "надо выводить
		// переменная это или константа") - scope.params отдельно ПЕРЕД
		// localsAt (тот сливает оба в одну карту без различения).
		for _, p := range scope.params {
			if p.Name == name {
				return codeBlock(fmt.Sprintf("параметр %s: %s", name, d.dispType(p.Type))), true
			}
		}
		if t, ok := localsAt(scope, line)[name]; ok {
			return codeBlock(fmt.Sprintf("переменная %s: %s", name, d.dispType(t))), true
		}
	}
	if fn, ok := d.funcs[name]; ok {
		block := withDoc(codeBlock(d.functionSignature(d.disp(fn.Name), fn.Params, fn.HasReturn, fn.ReturnType)), fn.Doc)
		return withThrows(block, d.throwsOf(throwsFuncKey(fn.Name))), true
	}
	if block, ok := d.hoverExternFunc(name); ok {
		return block, true
	}
	if block, ok := d.hoverExternEntity(name); ok {
		return block, true
	}
	if ent, ok := d.entities[name]; ok {
		kind := "сущность"
		if ent.IsStruct {
			kind = "структура"
		}
		if ent.IsAbstract {
			kind = "абстрактная " + kind
		}
		header := fmt.Sprintf("%s %s", kind, d.disp(ent.Name))
		if ctor, ok := d.entityCtorSignature(ent); ok {
			header += "\n" + ctor
		}
		// entDoc/builtinTypeDocs: сущность УЖЕ зарегистрирована в d.entities
		// (значит где-то в файле встретилась раньше места наведения) - но у
		// зарезервированных сущностей (internal/semantic/mutex.go,
		// semaphore.go, dbtx.go, ...) ir.Entity строится напрямую компилятором
		// и Doc никогда не заполняется (заполняется только для пользовательских
		// сущностей из doc-комментария, internal/semantic/analyzer.go
		// buildIR). Без этого отката ветка builtinTypeDocs ниже вообще не
		// достигается (она под "else"-условием d.entities[name] отсутствует),
		// и описание "Блокировка"/"Семафор"/"Транзакция"/... пропадает
		// целиком, как только сущность где-то использована в файле
		// (НАЙДЕННЫЙ БАГ - репортнуто пользователем).
		entDoc := ent.Doc
		if entDoc == "" {
			entDoc = builtinTypeDocs[name]
		}
		block := withDoc(codeBlock(header), entDoc)
		return withThrows(block, d.throwsOf(throwsCtorKey(ent.Name))), true
	}
	if doc, ok := builtinTypeDocs[name]; ok {
		header := name
		if ctor, ok := d.reservedEntityCtorSignature(name); ok {
			header = ctor
		}
		return codeBlock(header) + "\n\n" + doc, true
	}
	if block, ok := hoverBuiltinFunc(name); ok {
		return block, true
	}
	if block, ok := d.hoverEnumType(name); ok {
		return block, true
	}
	if block, ok := d.hoverSynonym(name); ok {
		return block, true
	}
	if block, ok := d.hoverModuleConst(name); ok {
		return block, true
	}
	if block, ok := d.hoverBareNameWordform(name, line); ok {
		return block, true
	}
	return d.hoverOwnMemberDecl(name, line)
}

// hoverOwnMemberDecl - курсор на имени поля/константы/метода СУЩНОСТИ НА
// ЕГО ЖЕ СТРОКЕ ОБЪЯВЛЕНИЯ ("имеет: имя: Тип", "константа ИМЯ = ...", сама
// строка "имя(...)" объявления метода) - не после "receiver.", поэтому ни
// одна ветка выше (те - про голое имя ВЕРХНЕГО уровня/локальное) его не
// находит вовсе, hoverMember (который умеет форматировать член сущности)
// ищется по РЕЦЕПИЕНТУ, а на самой строке объявления его нет. Запасной путь
// В САМОМ КОНЦЕ (редкий случай - что-то ДРУГОЕ уже совпало бы раньше, если
// бы могло) - перебирает сущности документа, ищет член с этим именем И
// ЭТОЙ строкой, затем форматирует его тем же hoverMember, что и обычный
// "receiver.член".
func (d *doc) hoverOwnMemberDecl(name string, line int) (string, bool) {
	for _, ed := range d.astEntities {
		ent := d.entities[ed.Name]
		if ent == nil {
			continue
		}
		for _, f := range ed.Fields {
			if f.Name == name && f.Line == line {
				return d.hoverMember(ent, name)
			}
		}
		for _, c := range ed.Constants {
			if c.Name == name && c.Line == line {
				return d.hoverMember(ent, name)
			}
		}
		for _, m := range ed.Methods {
			if m.Name == name && m.Line == line {
				return d.hoverMember(ent, name)
			}
		}
	}
	return "", false
}

// hoverBareNameWordform - запасной путь hoverBareName для имени, чья лемма
// объявления отличается от того, что написано под курсором (падежная форма
// сущности после "наследует"/"реализует", деепричастная форма функции,
// словоформа локальной переменной/параметра - см. wordform_definition.go):
// находит лемму и делегирует ОБЫЧНОМУ hoverBareName на неё - лемма
// буквально совпадёт с одной из веток выше, без повторения форматирования
// здесь. "", false - name не оказалось ни одной из трёх словоформ.
func (d *doc) hoverBareNameWordform(name string, line int) (string, bool) {
	if scope := d.scopeAt(line); scope != nil {
		if lemma, ok := d.localWordform(scope, line, name); ok {
			return d.hoverBareName(lemma, line)
		}
	}
	if lemma, ok := d.funcGerund(name); ok {
		return d.hoverBareName(lemma, line)
	}
	if lemma, ok := d.entityWordform(name); ok {
		return d.hoverBareName(lemma, line)
	}
	return "", false
}

// entityCtorSignature - "новый Тип(параметры)" для ent, показываемое рядом
// с самим объявлением при наведении на имя сущности - явный конструктор
// (ent.Ctor), если он есть; иначе, у структуры без ctor - её поля
// позиционно (единственная форма, которую неявный конструктор структуры
// принимает, см. internal/semantic/expr.go, analyzeNewExpr, ветка
// !sig.HasCtor && sig.IsStruct); иначе (сущность/класс без ctor) - "новый
// Тип()" совсем без аргументов (та же функция, ветка !sig.IsStruct - у
// класса без явного "конструктор" поля не задаются позиционно вообще, HasCtor
// НИКОГДА не наследуется от базовой сущности - internal/semantic/
// inheritance.go, applyInheritance - ent.Ctor == nil здесь означает ровно
// это, а не "проверить базовую"). "", false - абстрактная сущность (нельзя
// создать напрямую, показывать нечего).
func (d *doc) entityCtorSignature(ent *ir.Entity) (string, bool) {
	params, ok := ctorParamList(ent)
	if !ok {
		return "", false
	}
	// appendParamDefaults/ctorParamDefaults - те же самые (signaturehelp.go),
	// что и у "новый Тип(...)" в signature help - НАЙДЕННЫЙ БАГ (репортнуто
	// пользователем - "не вижу в ховере дефолтные значения хотя бы для
	// Запроса"): hover строил сигнатуру напрямую через d.functionSignature
	// (голые "имя: Тип" без "= значение"), signature help - через
	// appendParamDefaults - те же данные, два разных места сборки строки, и
	// только одно из двух подключало значения по умолчанию.
	labels := appendParamDefaults(d.paramLabels(params), d.ctorParamDefaults(ent))
	name := d.disp(ent.Name)
	return d.functionSignatureFromLabels(entityCtorKeyword(name)+" "+name, labels, false, ir.Type{}), true
}

// entityCtorKeyword - "новый"/"новая"/"новое"/"новые", согласованное по
// роду/числу с именем сущности name (morph.CompoundNounAgreement) - тот же
// принцип, что и сам язык требует от написанного текста (internal/parser/
// call.go, checkNewKeywordAgreement/morph.NewKeywordFor) - hover/signature
// help должны ПОКАЗЫВАТЬ ту же форму, которую парсер потребовал бы, а не
// голое "новый" для любого имени (репортнуто пользователем - "новая
// Точка(...)", не "новый Точка(...)"). Род/число не определились
// (CompoundNounAgreement, ok == false) - "новый" тем же permissive
// fallback, что и у самого языка.
func entityCtorKeyword(name string) string {
	gender, number, ok := morph.CompoundNounAgreement(name)
	if !ok {
		return "новый"
	}
	return morph.NewKeywordFor(gender, number)
}

// ctorParamList - параметры конструктора ent, тем же путём, что и
// entityCtorSignature (тут же выше) - вынесено отдельно, чтобы signature
// help (signaturehelp.go) могла показать параметры ПО ОТДЕЛЬНОСТИ (с
// подсветкой текущего), не готовой строкой целиком. "", false - у ent нет
// способа его создать через "новый" вовсе (абстрактная сущность/
// "Транзакция" - см. комментарии у соответствующих веток ниже).
func ctorParamList(ent *ir.Entity) ([]ir.Param, bool) {
	if ent.IsAbstract {
		return nil, false
	}
	if ent.IsTransaction {
		// "Транзакция" - нет "новый" вообще (см. package doc internal/
		// semantic/dbtx.go, IsTransaction в analyzeNewExpr) - значение
		// получить можно только параметром колбэка "начать_транзакцию".
		return nil, false
	}
	if ent.IsSemaphore {
		// "Семафор" - единственная системная сущность (наравне с
		// "Блокировкой" чуть ниже) без ast.EntityDecl/ent.Ctor, но, в
		// отличие от "Блокировки", с ОДНИМ обязательным параметром
		// конструктора (internal/semantic/semaphore.go) - общий путь ниже
		// (!ent.IsStruct -> "()") дал бы неверную сигнатуру без аргумента.
		return []ir.Param{{Name: "начальное_значение", Type: ir.TypeInt}}, true
	}
	if ent.Ctor != nil {
		return ent.Ctor.Params, true
	}
	if !ent.IsStruct {
		return nil, true
	}
	params := make([]ir.Param, len(ent.Fields))
	for i, f := range ent.Fields {
		params[i] = ir.Param{Name: f.Name, Type: f.Type}
	}
	return params, true
}

// hoverMember - name - поле/метод/константа ent (последний элемент цепочки
// "receiver.поле"/"receiver.метод").
func (d *doc) hoverMember(ent *ir.Entity, name string) (string, bool) {
	entName := d.disp(ent.Name)
	if ent.IsSemaphore && name == "захватить" {
		// "Семафор.захватить" - единственный из четырёх методов Блокировки/
		// Семафора с необязательным параметром (таймаут_мс,
		// HTTP highload пункт 3) - общий путь ниже (mutexMethodDocs, "()"
		// без аргументов) дал бы неверную сигнатуру, поэтому перехватывается
		// раньше него, отдельной готовой строкой (semaphoreAcquireDoc).
		return codeBlock(entName + "." + semaphoreAcquireDoc), true
	}
	if ent.IsMutex || ent.IsSemaphore {
		// "Блокировка"/"Семафор" - без ast.EntityDecl, ent.Methods у них
		// пуст (оба метода синтезированы backend'ом напрямую по
		// IsMutex/IsSemaphore, см. package doc internal/backend/cbackend/
		// mutex.go/semaphore.go) - обычный цикл по ent.Methods ниже никогда
		// не находит "захватить"/"освободить", поэтому наведение на них
		// нужно перехватить здесь же, канонической строкой.
		if doc, ok := mutexMethodDocs[name]; ok {
			return withDoc(codeBlock(fmt.Sprintf("%s.%s()", entName, name)), doc), true
		}
	}
	if ent.IsTransaction {
		// "Транзакция" - тот же пробел, что у "Блокировки"/"Семафора" выше
		// (нет ent.Methods, internal/semantic/dbtx.go), но её методы несут
		// содержательную сигнатуру - transactionMethodDocs хранит готовую
		// строку целиком, не только слово-описание.
		if doc, ok := transactionMethodDocs[name]; ok {
			return codeBlock(entName + "." + doc), true
		}
	}
	for _, f := range ent.Fields {
		if f.Name == name {
			block := withDoc(codeBlock(fmt.Sprintf("%s.%s: %s", entName, f.Name, d.dispType(f.Type))), f.Doc)
			return block, true
		}
	}
	for _, c := range ent.Constants {
		if c.Name == name {
			block := codeBlock(fmt.Sprintf("%s.%s: %s (константа)", entName, c.Name, d.dispType(c.Type)))
			if lit, ok := literalText(c.Value); ok {
				block += fmt.Sprintf("\n\nЗначение: `%s`", lit)
			}
			block = withDoc(block, c.Doc)
			return block, true
		}
	}
	for _, m := range ent.Methods {
		if m.Name == name {
			doc := d.resolvedMethodDoc(ent, name, m.Doc)
			block := withDoc(codeBlock(d.functionSignature(entName+"."+m.Name, m.Params, m.HasReturn, m.ReturnType)), doc)
			return withThrows(block, d.throwsOf(throwsMethodKey(ent.Name, m.Name))), true
		}
	}
	return "", false
}

// literalText - текстовое представление значения константы, если оно уже
// разрешилось до простого литерала (типичный случай - "константа ИМЯ =
// 100") - "", false для любого более сложного выражения (арифметика,
// обращение к другой константе, вызов функции - resolveEntityConstant
// разрешает "любое выражение", но hover показывает значение только там,
// где это один литерал, не пытаясь заново вычислить/напечатать
// произвольное выражение).
func literalText(v ir.Expr) (string, bool) {
	switch v := v.(type) {
	case *ir.IntLiteral:
		return strconv.FormatInt(v.Value, 10), true
	case *ir.FloatLiteral:
		return strconv.FormatFloat(v.Value, 'g', -1, 64), true
	case *ir.StringLiteral:
		return strconv.Quote(v.Value), true
	case *ir.BoolLiteral:
		if v.Value {
			return "истина", true
		}
		return "ложь", true
	case *ir.CharLiteral:
		return "'" + string(v.Value) + "'", true
	default:
		return "", false
	}
}

// functionSignature - "функция имя(параметр: Тип, ...): ТипРезультата" -
// без "функция" для методов (entityMethodName уже несёт "Тип.метод"). name
// уже должно быть пропущено через d.disp вызывающей стороной (сама
// функция/метод могла быть найдена по алиасу, но название параметров ей не
// принадлежит явно - только типы параметров/результата нуждаются в
// dispType здесь).
func (d *doc) functionSignature(name string, params []ir.Param, hasReturn bool, returnType ir.Type) string {
	return d.functionSignatureFromLabels(name, d.paramLabels(params), hasReturn, returnType)
}

// functionSignatureFromLabels - как functionSignature, но принимает уже
// готовые строки параметров (paramLabels ниже, signaturehelp.go) вместо
// []ir.Param - используется signatureHelp, которому нужны те же строки И
// целиком в Label, И по отдельности в ParamLabels (с добавленным " =
// значение" у параметров со значением по умолчанию, см.
// appendParamDefaults) - построение из ОДНОГО и того же слайса гарантирует,
// что каждый ParamLabels[i] останется буквальной подстрокой Label (нужно
// клиенту, VS Code ищет подстроку в Label, чтобы подсветить активный
// параметр).
func (d *doc) functionSignatureFromLabels(name string, paramLabels []string, hasReturn bool, returnType ir.Type) string {
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(paramLabels, ", "))
	if hasReturn {
		sig += ": " + d.dispType(returnType)
	}
	return sig
}

func codeBlock(code string) string {
	return "```rech\n" + code + "\n```"
}

// withDoc добавляет текст doc-комментария (обрезанный по краям пробел)
// под блоком кода - пусто, если Doc == "" (у объявления не было
// doc-комментария).
func withDoc(block, doc string) string {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return block
	}
	return block + "\n\n" + doc
}
