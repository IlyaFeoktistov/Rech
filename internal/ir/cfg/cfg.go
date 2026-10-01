// Package cfg строит граф потока управления (control flow graph) тела
// функции/метода/лямбды - настоящий граф базовых блоков, не текстовая
// эвристика по последней инструкции блока (какой была прежняя проверка
// "вернуть на каждом пути" в internal/semantic, см. package doc там же).
// Единственный источник истины про "какой код физически недостижим" и "все
// ли живые пути тела заканчиваются вернуть/выбросить, а не проваливаются в
// конец нормально" - обе проверки сводятся к одной задаче (достижимость в
// графе, см. Graph.Reachable), а не к двум параллельным рекурсивным
// предикатам.
//
// Rech - язык со структурным потоком управления (нет goto/меток, только
// вложенные если/циклы/попытаться с прервать/продолжить по числовой
// глубине вложенности) - граф строится ОДНИМ плоским проходом (Build) без
// отдельных подграфов на каждую вложенную конструкцию, break/continue
// резолвятся через стек целевых блоков текущих циклов на момент построения
// (см. build.go).
package cfg

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// Block - один базовый блок: прямая последовательность инструкций без
// внутреннего ветвления (если/пока/для/попытаться/прервать/продолжить/
// вернуть/выбросить сами присутствуют как ОБЫЧНЫЕ элементы Stmts там, где
// это просто "инструкция в блоке", но каждая из них также заканчивает блок
// - см. build.go) - Succs пуст, если поток отсюда не продолжается никуда
// (гарантированный "тупик" - вернуть/выбросить/прервать наружу цикла без
// дальнейшего кода, бесконечный цикл без прервать).
type Block struct {
	ID    int
	Stmts []ir.Stmt
	Succs []int
	// Loop - у заголовка цикла (пустой блок, куда возвращается тело
	// после каждой итерации) - сама инструкция цикла: в этой точке заново
	// вычисляется условие "пока"/"для ;;" и присваивается переменная
	// "для ... в"/"для ... от". nil у всех остальных блоков.
	Loop ir.Stmt
}

// Graph - граф базовых блоков ОДНОГО тела (функции/метода/лямбды), см.
// Build.
type Graph struct {
	Blocks []*Block
	// Entry - ID первого блока (начало тела).
	Entry int
	// Exit - ID синтетического узла "нормальное завершение тела целиком"
	// (провал в конец списка инструкций верхнего уровня без вернуть/
	// выбросить/бесконечного цикла) - без единого входящего ребра, если
	// тело гарантированно расходится на каждом живом пути (см. Build).
	// Сам по себе не несёт инструкций (Stmts пуст).
	Exit int
}

func (g *Graph) block(id int) *Block { return g.Blocks[id] }

// Reachable - BFS от g.Entry по Succs - множество ID блоков, реально
// достижимых от начала тела. Циклы (обычные пока/для - не единственный
// источник рёбер "назад" в этом графе, но единственный, который создаёт
// настоящие циклы в самом графе) безопасны - каждый блок посещается не
// больше одного раза.
func (g *Graph) Reachable() map[int]bool {
	reach := make(map[int]bool, len(g.Blocks))
	reach[g.Entry] = true
	queue := []int{g.Entry}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, succ := range g.block(id).Succs {
			if !reach[succ] {
				reach[succ] = true
				queue = append(queue, succ)
			}
		}
	}
	return reach
}

// FallsThrough - true, если хотя бы один живой (достижимый от начала) путь
// тела заканчивается нормальным провалом в конец (не вернуть/выбросить) -
// заменяет прежнюю internal/semantic.endsWithReturnValue: "не все пути
// функции/метода/лямбды с объявленным типом результата заканчиваются
// вернуть <значение>" - именно тогда, когда g.Exit достижим.
func (g *Graph) FallsThrough() bool {
	return g.Reachable()[g.Exit]
}

// UnreachableLine - номер строки первой (по возрастанию номера строки, а не
// по порядку построения блоков - порядок блоков не обязан совпадать с
// текстовым порядком кода при ветвлении) инструкции, недостижимой от начала
// тела - 0, false, если таких нет. Пустые (синтетические) блоки без
// инструкций (например, само g.Exit) не считаются - недостижимость точки
// графа, в которой ничего не написано пользователем, не диагностика.
//
// "окончательно" внутри попытаться/поймать (build.go, emitTry) строится
// ДВУМЯ независимыми копиями блоков с ОДНИМИ И ТЕМИ ЖЕ указателями ir.Stmt
// внутри (одна копия лежит на настоящем пути после Body/CatchBody, вторая -
// только ради достижимости изнутри раннего выхода) - когда единственный
// живой путь к "окончательно" идёт через раннюю копию (типичный случай -
// тело всегда завершается вернуть/выбросить), блок-копия НА НАСТОЯЩЕМ пути
// формально недостижима сама по себе, хотя те же инструкции реально
// исполняются - поэтому недостижимость проверяется на уровне указателя
// ir.Stmt (встречается хотя бы в одном достижимом блоке - живая), а не на
// уровне отдельного блока напрямую.
func (g *Graph) UnreachableLine() (int, bool) {
	reach := g.Reachable()
	live := g.liveStmts()
	line, found := 0, false
	for _, b := range g.Blocks {
		if reach[b.ID] || len(b.Stmts) == 0 || live[b.Stmts[0]] {
			continue
		}
		l := ir.LineOf(b.Stmts[0])
		if l == 0 {
			continue
		}
		if !found || l < line {
			line, found = l, true
		}
	}
	return line, found
}

// liveStmts - инструкции, встречающиеся хотя бы в одном достижимом блоке
// (уровень указателя ir.Stmt - см. UnreachableLine про "окончательно").
func (g *Graph) liveStmts() map[ir.Stmt]bool {
	reach := g.Reachable()
	live := make(map[ir.Stmt]bool)
	for _, b := range g.Blocks {
		if !reach[b.ID] {
			continue
		}
		for _, s := range b.Stmts {
			live[s] = true
		}
	}
	return live
}

// UnreachableRegions - строки начала недостижимых кусков body, по которому
// построен g: в каждом списке инструкций (тело и вложенные блоки) - первая
// недостижимая инструкция; всё после неё до конца этого же блока тоже
// недостижимо (инструкции списка исполняются по порядку), так что одна
// строка - один кусок. Внутрь недостижимой инструкции обход не заходит -
// её вложенные блоки входят в тот же кусок.
func (g *Graph) UnreachableRegions(body []ir.Stmt) []int {
	live := g.liveStmts()
	var lines []int
	var walk func(stmts []ir.Stmt)
	walk = func(stmts []ir.Stmt) {
		for _, s := range stmts {
			if !stmtLive(s, live) {
				if l := ir.LineOf(s); l > 0 {
					lines = append(lines, l)
				}
				return
			}
			for _, nested := range nestedBodies(s) {
				walk(nested)
			}
		}
	}
	walk(body)
	return lines
}

// stmtLive - s исполняется хотя бы на одном пути. GerundStmt/BlockStmt
// сами в блоки не попадают (emitStmt разворачивает их тело на месте) -
// живы, если жива хоть одна их инструкция; пустые - живы.
func stmtLive(s ir.Stmt, live map[ir.Stmt]bool) bool {
	var body []ir.Stmt
	switch s := s.(type) {
	case *ir.GerundStmt:
		body = s.Body
	case *ir.BlockStmt:
		body = s.Body
	default:
		return live[s]
	}
	if len(body) == 0 {
		return true
	}
	for _, c := range body {
		if stmtLive(c, live) {
			return true
		}
	}
	return false
}

// nestedBodies - списки инструкций, вложенные прямо в s.
func nestedBodies(s ir.Stmt) [][]ir.Stmt {
	switch s := s.(type) {
	case *ir.IfStmt:
		return [][]ir.Stmt{s.Then, s.Else}
	case *ir.WhileStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.ForStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.ForEachStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.CStyleForStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.TryStmt:
		return [][]ir.Stmt{s.Body, s.CatchBody, s.Finally}
	case *ir.GerundStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.BlockStmt:
		return [][]ir.Stmt{s.Body}
	}
	return nil
}
