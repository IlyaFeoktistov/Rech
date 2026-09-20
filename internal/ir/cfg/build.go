package cfg

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// loopTarget - куда ведут "прервать"/"продолжить" для одного уровня
// вложенности цикла на момент построения - индексация стека loops зеркалит
// глубину вложенности циклов, которую уже проверил internal/semantic
// (BreakStmt.Depth/ContinueStmt.Depth гарантированно укладываются в
// реальную вложенность в этой точке - здесь это не перепроверяется).
type loopTarget struct {
	breakID    int
	continueID int
}

type builder struct {
	g     *Graph
	loops []loopTarget
}

func (b *builder) newBlock() *Block {
	blk := &Block{ID: len(b.g.Blocks)}
	b.g.Blocks = append(b.g.Blocks, blk)
	return blk
}

// Build строит граф потока управления для body - тела функции/метода/
// лямбды ЦЕЛИКОМ (не произвольного вложенного блока - для вложенных
// конструкций используется (*builder).emitBlock рекурсивно изнутри, тот же
// плоский граф). См. package doc Graph.Exit.
func Build(body []ir.Stmt) *Graph {
	g := &Graph{}
	b := &builder{g: g}

	entry := b.newBlock()
	g.Entry = entry.ID
	cur := b.emitBlock(entry, body)

	exit := b.newBlock()
	g.Exit = exit.ID
	if cur != nil {
		cur.Succs = append(cur.Succs, exit.ID)
	}
	return g
}

// emitBlock дописывает stmts в cur (начиная новые блоки на каждой
// развилке/цикле) и возвращает блок, в который естественно продолжится код
// ПОСЛЕ stmts при нормальном завершении - nil, если stmts гарантированно
// расходится (последний живой путь заканчивается вернуть/выбросить/
// прервать/продолжить/бесконечным циклом без прервать наружу). cur == nil
// на входе (для statement'а сразу после расходящейся инструкции той же
// последовательности) - создаёт для него СВЕЖИЙ блок без единого входящего
// ребра, то есть заведомо недостижимый (Graph.UnreachableLine потом его
// находит).
func (b *builder) emitBlock(cur *Block, stmts []ir.Stmt) *Block {
	for _, s := range stmts {
		if cur == nil {
			cur = b.newBlock()
		}
		cur = b.emitStmt(cur, s)
	}
	return cur
}

func (b *builder) emitStmt(cur *Block, s ir.Stmt) *Block {
	switch s := s.(type) {
	case *ir.ReturnStmt:
		cur.Stmts = append(cur.Stmts, s)
		return nil
	case *ir.ThrowStmt:
		cur.Stmts = append(cur.Stmts, s)
		return nil
	case *ir.BreakStmt:
		cur.Stmts = append(cur.Stmts, s)
		t := b.loops[len(b.loops)-s.Depth]
		cur.Succs = append(cur.Succs, t.breakID)
		return nil
	case *ir.ContinueStmt:
		cur.Stmts = append(cur.Stmts, s)
		t := b.loops[len(b.loops)-s.Depth]
		cur.Succs = append(cur.Succs, t.continueID)
		return nil
	case *ir.IfStmt:
		return b.emitIf(cur, s)
	case *ir.WhileStmt:
		return b.emitLoop(cur, s, s.Cond, s.Body)
	case *ir.ForStmt:
		return b.emitUnconditionalLoop(cur, s, s.Body)
	case *ir.ForEachStmt:
		return b.emitUnconditionalLoop(cur, s, s.Body)
	case *ir.CStyleForStmt:
		return b.emitCStyleFor(cur, s)
	case *ir.TryStmt:
		return b.emitTry(cur, s)
	case *ir.GerundStmt:
		// Маркер новой области видимости - для потока управления
		// прозрачен (см. package doc ir.GerundStmt) - его Body исполняется
		// прямо на месте, тем же блоком.
		return b.emitBlock(cur, s.Body)
	case *ir.BlockStmt:
		// Тем же принципом, что GerundStmt выше - десахаренный "выбрать"
		// (internal/semantic/switch.go) не вносит собственного ветвления,
		// им является цепочка "если", которую он несёт внутри Body.
		return b.emitBlock(cur, s.Body)
	default:
		cur.Stmts = append(cur.Stmts, s)
		return cur
	}
}

// emitIf - "если <условие> { Then } [иначе { Else }]". Без явного "иначе" -
// ложная ветвь всегда "проваливается" прямо в продолжение (cur - тот же
// блок, где стояло условие) - в отличие от прежней internal/semantic.
// blockDiverges/endsWithReturnValue, которые требовали ЯВНОГО "иначе",
// чтобы вообще засчитать эту ветвь: здесь она честно смоделирована - не
// нужно притворяться, что она есть, чтобы дать верный ответ.
func (b *builder) emitIf(cur *Block, s *ir.IfStmt) *Block {
	cur.Stmts = append(cur.Stmts, s)

	thenEntry := b.newBlock()
	cur.Succs = append(cur.Succs, thenEntry.ID)
	thenExit := b.emitBlock(thenEntry, s.Then)

	if s.Else == nil {
		join := b.newBlock()
		cur.Succs = append(cur.Succs, join.ID)
		if thenExit != nil {
			thenExit.Succs = append(thenExit.Succs, join.ID)
		}
		return join
	}

	elseEntry := b.newBlock()
	cur.Succs = append(cur.Succs, elseEntry.ID)
	elseExit := b.emitBlock(elseEntry, s.Else)

	if thenExit == nil && elseExit == nil {
		return nil
	}
	join := b.newBlock()
	if thenExit != nil {
		thenExit.Succs = append(thenExit.Succs, join.ID)
	}
	if elseExit != nil {
		elseExit.Succs = append(elseExit.Succs, join.ID)
	}
	return join
}

// isAlwaysTrue - true у "пока истина"/"для (;;)" (Cond - буквальный
// логический литерал "истина", не произвольное условие, которое СЛУЧАЙНО
// всегда истинно во время исполнения - тот же узкий, но честный принцип,
// что был у прежней эвристики, не полный constant folding). nil (пустое
// условие "для ;; {...}", ir.CStyleForStmt.Cond) - тоже всегда истина, как
// и в C, проверяется отдельно вызывающим кодом.
func isAlwaysTrue(cond ir.Expr) bool {
	lit, ok := cond.(*ir.BoolLiteral)
	return ok && lit.Value
}

// emitLoop - "пока <cond> { Body }": header - блок проверки условия,
// продолжает в bodyEntry всегда и в after ("прервать"-цель), КРОМЕ
// isAlwaysTrue(cond) - тогда ребра в after у header нет вовсе, after
// достижим ТОЛЬКО через явный "прервать" изнутри Body - естественно
// повторяет то самое "пока истина без прервать - расходится", которое
// прежняя эвристика распознавала отдельным особым случаем, здесь это просто
// следствие общей конструкции графа. self - сама инструкция цикла (для
// Stmts блока-предшественника, только для точности диагностики).
func (b *builder) emitLoop(cur *Block, self ir.Stmt, cond ir.Expr, body []ir.Stmt) *Block {
	cur.Stmts = append(cur.Stmts, self)
	header := b.newBlock()
	cur.Succs = append(cur.Succs, header.ID)

	bodyEntry := b.newBlock()
	after := b.newBlock()
	header.Succs = append(header.Succs, bodyEntry.ID)
	if !isAlwaysTrue(cond) {
		header.Succs = append(header.Succs, after.ID)
	}

	b.loops = append(b.loops, loopTarget{breakID: after.ID, continueID: header.ID})
	bodyExit := b.emitBlock(bodyEntry, body)
	b.loops = b.loops[:len(b.loops)-1]
	if bodyExit != nil {
		bodyExit.Succs = append(bodyExit.Succs, header.ID)
	}
	return after
}

// emitUnconditionalLoop - "для <имя> от..до"/"для <имя> в <коллекция>" -
// диапазон/коллекция не несёт статически известного "всегда хотя бы одна
// итерация" (в отличие от буквального "истина" у emitLoop) - header всегда
// ведёт И в тело, И в after, независимо от вида цикла.
func (b *builder) emitUnconditionalLoop(cur *Block, self ir.Stmt, body []ir.Stmt) *Block {
	cur.Stmts = append(cur.Stmts, self)
	header := b.newBlock()
	cur.Succs = append(cur.Succs, header.ID)

	bodyEntry := b.newBlock()
	after := b.newBlock()
	header.Succs = append(header.Succs, bodyEntry.ID, after.ID)

	b.loops = append(b.loops, loopTarget{breakID: after.ID, continueID: header.ID})
	bodyExit := b.emitBlock(bodyEntry, body)
	b.loops = b.loops[:len(b.loops)-1]
	if bodyExit != nil {
		bodyExit.Succs = append(bodyExit.Succs, header.ID)
	}
	return after
}

// emitCStyleFor - "для <init>; <cond>; <post> { Body }" - "продолжить" ведёт
// в post (шаг выполняется, ПОТОМ снова проверяется условие), не напрямую в
// header, ровно как в C - единственное отличие от emitLoop по форме графа
// (там "продолжить" ведёт прямо в header, там нет отдельного шага).
func (b *builder) emitCStyleFor(cur *Block, s *ir.CStyleForStmt) *Block {
	cur.Stmts = append(cur.Stmts, s)
	if s.Init != nil {
		cur.Stmts = append(cur.Stmts, s.Init)
	}
	header := b.newBlock()
	cur.Succs = append(cur.Succs, header.ID)

	bodyEntry := b.newBlock()
	after := b.newBlock()
	always := s.Cond == nil || isAlwaysTrue(s.Cond)
	header.Succs = append(header.Succs, bodyEntry.ID)
	if !always {
		header.Succs = append(header.Succs, after.ID)
	}

	post := b.newBlock()
	if s.Post != nil {
		post.Stmts = append(post.Stmts, s.Post)
	}
	post.Succs = append(post.Succs, header.ID)

	b.loops = append(b.loops, loopTarget{breakID: after.ID, continueID: post.ID})
	bodyExit := b.emitBlock(bodyEntry, s.Body)
	b.loops = b.loops[:len(b.loops)-1]
	if bodyExit != nil {
		bodyExit.Succs = append(bodyExit.Succs, post.ID)
	}
	return after
}

// emitTry - "попытаться { Body } [поймать(...) { CatchBody }] [окончательно
// { Finally }]". Цепочка из нескольких "поймать" уже пришла из
// internal/parser ВЛОЖЕННЫМИ узлами TryStmt (первый написанный "поймать" -
// самый внутренний - см. package doc ast.TryStmt) - emitTry не знает про
// цепочку вовсе, каждый уровень обрабатывается этой же функцией по
// отдельности, вложенность приходит сама собой через обычную рекурсию
// emitBlock(Body).
//
// catchEntry достижим БЕЗУСЛОВНО от начала попытки (ребро от cur, не только
// от явных ir.ThrowStmt внутри Body) - Rech не отслеживает статически,
// какая инструкция может кинуть исключение/панику (индекс за границей,
// несовпадение "как" и т. п. - см. ir.TryStmt.CatchPanic) - без этого
// ослабления обычный "поймать" без единого явного "выбросить" в Body ложно
// считался бы недостижимым кодом.
//
// "окончательно" (если есть) строится ДВУМЯ независимыми копиями блоков -
// см. комментарий у normalFinallyEntry/soundnessFinallyEntry ниже: одна
// копия лежит на настоящем пути "нормальное завершение Body/CatchBody",
// вторая нужна только для достижимости собственных инструкций
// "окончательно" на путях раннего выхода. Единственный узел на двоих был бы
// НЕВЕРЕН - слил бы "продолжение после всей попытки" (только у первой
// копии) с "куда-то там, неважно куда" (вторая копия) в одно и то же ребро.
func (b *builder) emitTry(cur *Block, s *ir.TryStmt) *Block {
	cur.Stmts = append(cur.Stmts, s)

	bodyEntry := b.newBlock()
	cur.Succs = append(cur.Succs, bodyEntry.ID)

	var catchEntry *Block
	if !s.NoCatch {
		catchEntry = b.newBlock()
		cur.Succs = append(cur.Succs, catchEntry.ID)
	}

	bodyExit := b.emitBlock(bodyEntry, s.Body)

	var catchExit *Block
	if !s.NoCatch {
		catchExit = b.emitBlock(catchEntry, s.CatchBody)
	}

	join := b.newBlock()
	if bodyExit != nil {
		bodyExit.Succs = append(bodyExit.Succs, join.ID)
	}
	if catchExit != nil {
		catchExit.Succs = append(catchExit.Succs, join.ID)
	}

	if len(s.Finally) == 0 {
		return join
	}

	// "окончательно" исполняется на КАЖДОМ пути (см. package doc) - граф
	// строит его тело ДВАЖДЫ, двумя независимыми копиями блоков: одна - на
	// настоящем пути нормального завершения (join -> ... -> продолжение
	// после всей конструкции целиком, её "падение в конец" - реальный
	// afterTry, возвращается вызывающему коду ниже); вторая - только ради
	// достижимости СОБСТВЕННЫХ инструкций "окончательно" на путях раннего
	// "вернуть"/"выбросить"/"прервать"/"продолжить" изнутри Body/CatchBody
	// (её "падение в конец" НИКУДА не подключено - в реальности в этом
	// случае исполнение продолжает тот ранний путь, а не то, что написано
	// после "попытаться", но КУДА именно - не нужно ни одной из двух
	// проверок этого пакета, см. package doc emitTry). Без второй копии
	// путь "тело всегда вернуть, окончательно нормально завершается" ложно
	// считался бы падающим В ПРОДОЛЖЕНИЕ ПОСЛЕ ПОПЫТКИ - его нет, единственный
	// finallyEntry на двоих склеил бы два разных смысла одного и того же
	// "нормально завершилось" ребра (найдено юнит-тестом на этом самом
	// сценарии).
	normalFinallyEntry := b.newBlock()
	join.Succs = append(join.Succs, normalFinallyEntry.ID)
	normalFinallyExit := b.emitBlock(normalFinallyEntry, s.Finally)

	soundnessFinallyEntry := b.newBlock()
	cur.Succs = append(cur.Succs, soundnessFinallyEntry.ID)
	b.emitBlock(soundnessFinallyEntry, s.Finally)

	return normalFinallyExit
}
