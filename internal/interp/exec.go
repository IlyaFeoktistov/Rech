package interp

import (
	"fmt"
	"os"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// execStmt исполняет одну инструкцию - см. execBlock (interp.go) про то, как
// ctrlReturn прерывает выполнение вверх по стеку блоков/вызову.
//
// Строка ТЕКУЩЕЙ инструкции обновляет *cc.curLine ДО её исполнения - см.
// package doc internal/interp/trace.go - если внутри неё (вложенным
// вызовом или напрямую) случится паника/исключение, кадр этой активации
// уже покажет правильную строку.
func (in *Interp) execStmt(s ir.Stmt, env *Env, cc *callCtx) (ctrl, error) {
	if line := ir.LineOf(s); line > 0 {
		*cc.curLine = line
		cc.dbg.onStatement(env, line)
	}
	switch s := s.(type) {
	case *ir.PrintStmt:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		in.printValue(v)
		return ctrl{}, nil

	case *ir.ShowStmt:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		fmt.Fprintln(in.out, in.showValue(v))
		return ctrl{}, nil

	case *ir.VarDecl:
		if s.IsStatic {
			// "статическая переменная" - значение вычисляется и сохраняется
			// РОВНО один раз за всё время работы программы (см. package doc
			// Interp.staticLocals) - при повторном достижении этой
			// инструкции (следующий вызов функции/следующая итерация
			// цикла) уже вычисленное значение просто переносится в текущий
			// Env, инициализатор s.Value не выполняется заново.
			if v, ok := in.staticLocals[s.StaticID]; ok {
				env.declare(s.Name, v)
			} else {
				v, err := in.evalExpr(s.Value, env, cc)
				if err != nil {
					return ctrl{}, err
				}
				cv := in.cloneValue(v)
				in.staticLocals[s.StaticID] = cv
				env.declare(s.Name, cv)
			}
			env.declareStatic(s.Name, s.StaticID)
			return ctrl{}, nil
		}
		if s.Value == nil {
			// Объявлена без значения - присваивание до первого чтения
			// гарантирует семантика.
			if ent, ok := in.entities[s.DeclType.Entity]; ok && s.DeclType.Kind == ir.KindEntity && ent.IsStruct {
				// Структура заполняется по полям ("п.a = 1").
				v, err := in.bareStruct(ent, s.DeclType.Entity, env, cc)
				if err != nil {
					return ctrl{}, err
				}
				env.declare(s.Name, v)
				return ctrl{}, nil
			}
			env.declare(s.Name, in.zeroFieldValue(s.DeclType))
			return ctrl{}, nil
		}
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		env.declare(s.Name, in.cloneValue(v))
		return ctrl{}, nil

	case *ir.Assign:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		cv := in.cloneValue(v)
		env.set(s.Name, cv)
		if id, ok := env.lookupStaticID(s.Name); ok {
			in.staticLocals[id] = cv
		}
		return ctrl{}, nil

	case *ir.NullCoalesceAssignStmt:
		cur, _ := env.get(s.Name)
		if cur.Nullable != nil {
			return ctrl{}, nil
		}
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		cv := in.cloneValue(v)
		env.set(s.Name, cv)
		if id, ok := env.lookupStaticID(s.Name); ok {
			in.staticLocals[id] = cv
		}
		return ctrl{}, nil

	case *ir.StaticFieldAssign:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if in.staticFields[s.Entity] == nil {
			in.staticFields[s.Entity] = map[string]Value{}
		}
		in.staticFields[s.Entity][s.Field] = in.cloneValue(v)
		return ctrl{}, nil

	case *ir.FieldAssign:
		if root, ok := derefRoot(s.Receiver); ok {
			return in.execFieldAssignThroughPointer(s, root, env, cc)
		}
		recv, err := in.evalExpr(s.Receiver, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		recv.Obj.setField(s.Field, in.cloneValue(v))
		return ctrl{}, nil

	case *ir.ArrayAssignStmt:
		return in.execArrayAssignStmt(s, env, cc)
	case *ir.DerefAssignStmt:
		return in.execDerefAssignStmt(s, env, cc)

	case *ir.ParentCallStmt:
		baseEnt := in.entities[s.BaseEntity]
		args := make([]Value, len(s.Args))
		for i, a := range s.Args {
			v, err := in.evalExpr(a, env, cc)
			if err != nil {
				return ctrl{}, err
			}
			args[i] = v
		}
		// callMethod - тот же путь, что и обычный метод: заводит новый Env
		// для параметров конструктора базовой сущности, но исполняет его
		// тело на ТОМ ЖЕ cc.this (тот же *entityObj, ещё строящийся) - см.
		// package doc ir.ParentCallStmt. Возвращаемое значение
		// (ctrlReturn от голого "вернуть" внутри тела базового конструктора,
		// если оно есть) не несёт смысла для конструктора и отбрасывается,
		// как и в самой callMethod для метода без объявленного результата.
		if _, err := in.callMethod(baseEnt.Ctor, cc.this, args, cc.dbg); err != nil {
			return ctrl{}, err
		}
		return ctrl{}, nil

	case *ir.BecomeStmt:
		// Переход уже структурно проверен семантическим анализом
		// (объявленное состояние сущности-получателя) - здесь только
		// отмечает его установленным в рантайм-хранилище (entityObj.states),
		// которое читает StateCheckExpr там, где established не смог
		// доказать переход на этапе компиляции.
		recv, err := in.evalExpr(s.Receiver, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		recv.Obj.setState(s.State)
		return ctrl{}, nil

	case *ir.AssertStmt:
		cond, err := in.evalExpr(s.Cond, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if !cond.B {
			if s.Message != nil {
				msg, err := in.evalExpr(s.Message, env, cc)
				if err != nil {
					return ctrl{}, err
				}
				panicRuntimeAs(rtmsg.PanicAssertion, "%s: %s", rtmsg.AssertFailed(s.Line), msg.S)
			}
			panicRuntimeAs(rtmsg.PanicAssertion, "%s", rtmsg.AssertFailed(s.Line))
		}
		return ctrl{}, nil

	case *ir.IfStmt:
		cond, err := in.evalExpr(s.Cond, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if cond.B {
			return in.execBlock(s.Then, newEnv(env), cc)
		}
		if s.Else != nil {
			return in.execBlock(s.Else, newEnv(env), cc)
		}
		return ctrl{}, nil

	case *ir.GerundStmt:
		// Новый Env - только чтобы "результат" (первый элемент Body, см.
		// package doc ir.GerundStmt) не была видна снаружи этого блока, тем
		// же принципом, что и у если/пока.
		return in.execBlock(s.Body, newEnv(env), cc)

	case *ir.BlockStmt:
		return in.execBlock(s.Body, newEnv(env), cc)

	case *ir.WhileStmt:
		for {
			cond, err := in.evalExpr(s.Cond, env, cc)
			if err != nil {
				return ctrl{}, err
			}
			if !cond.B {
				return ctrl{}, nil
			}
			c, err := in.execBlock(s.Body, newEnv(env), cc)
			if err != nil {
				return ctrl{}, err
			}
			if out, ok := handleLoopCtrl(c); !ok {
				return out, nil
			}
			// ctrlContinue (depth<=1)/ctrlNone - следующая итерация,
			// повторная проверка условия.
		}

	case *ir.ForStmt:
		from, err := in.evalExpr(s.From, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		to, err := in.evalExpr(s.To, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if s.TypedVar() {
			return in.execTypedRangeFor(s, from, to, env, cc)
		}
		for i := from.I; i <= to.I; i++ {
			loopEnv := newEnv(env)
			loopEnv.declare(s.Var, intValue(i))
			c, err := in.execBlock(s.Body, loopEnv, cc)
			if err != nil {
				return ctrl{}, err
			}
			if out, ok := handleLoopCtrl(c); !ok {
				return out, nil
			}
		}
		return ctrl{}, nil

	case *ir.ForEachStmt:
		collV, err := in.evalExpr(s.Collection, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if s.Kind == ir.ForEachFile {
			return in.execForEachFileLine(collV.File, s, env, cc)
		}
		return in.execForEachCollection(collV.Obj.coll, s, env, cc)

	case *ir.CStyleForStmt:
		loopEnv := newEnv(env)
		if s.Init != nil {
			v, err := in.evalExpr(s.Init.Value, loopEnv, cc)
			if err != nil {
				return ctrl{}, err
			}
			loopEnv.declare(s.Init.Name, in.cloneValue(v))
		}
		for {
			if s.Cond != nil {
				cond, err := in.evalExpr(s.Cond, loopEnv, cc)
				if err != nil {
					return ctrl{}, err
				}
				if !cond.B {
					return ctrl{}, nil
				}
			}
			c, err := in.execBlock(s.Body, newEnv(loopEnv), cc)
			if err != nil {
				return ctrl{}, err
			}
			if out, ok := handleLoopCtrl(c); !ok {
				return out, nil
			}
			// ctrlContinue (depth<=1)/ctrlNone - Post всё равно исполняется,
			// ровно как в C, ПЕРЕД повторной проверкой условия.
			if s.Post != nil {
				if _, err := in.execStmt(s.Post, loopEnv, cc); err != nil {
					return ctrl{}, err
				}
			}
		}

	case *ir.BreakStmt:
		return ctrl{kind: ctrlBreak, depth: s.Depth}, nil

	case *ir.ContinueStmt:
		return ctrl{kind: ctrlContinue, depth: s.Depth}, nil

	case *ir.ReturnStmt:
		if s.Value == nil {
			return ctrl{kind: ctrlReturn}, nil
		}
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		return ctrl{kind: ctrlReturn, value: in.cloneValue(v)}, nil

	case *ir.ThrowStmt:
		return in.execThrowStmt(s, env, cc)

	case *ir.TryStmt:
		return in.execTryStmt(s, env, cc)

	case *ir.ExprStmt:
		var e ir.Expr = s.Call
		switch {
		case s.MethodCall != nil:
			e = s.MethodCall
		case s.StaticMethodCall != nil:
			e = s.StaticMethodCall
		case s.IncDec != nil:
			e = s.IncDec
		case s.Await != nil:
			e = s.Await
		case s.CallValue != nil:
			e = s.CallValue
		}
		_, err := in.evalExpr(e, env, cc)
		return ctrl{}, err

	default:
		panicRuntime("исполнение: инструкция %T не поддерживается интерпретатором", s)
		return ctrl{}, nil
	}
}

// execForEachFileLine - "для <строка> в <Файл>" (в отличие от "для x в
// Список<T>" выше - файл не индексируемый и не известной заранее длины,
// поэтому это отдельная, потоковая форма цикла, а не общий механизм).
// bufio.NewReader читает не более чем нужно за раз - файл не загружается в
// память целиком, в отличие от builtins.All "прочитать" с фиксированным
// max. Пустая строка - настоящая пустая строка файла (Логическое ok здесь
// не нужно - в отличие от cbackend.rech_read_line_from_file, у Go
// bufio.Reader.ReadString уже есть err == io.EOF как отдельный от значения
// сигнал конца файла). Последняя строка без завершающего переноса строки
// - ReadString возвращает её вместе с io.EOF (не пустой err без данных) -
// тело цикла всё равно должно выполниться на ней один раз, прежде чем
// цикл завершится.
func (in *Interp) execForEachFileLine(f *os.File, s *ir.ForEachStmt, env *Env, cc *callCtx) (ctrl, error) {
	reader, release := in.fileLineReader(f, f)
	defer release()
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && line == "" {
			return ctrl{}, nil
		}
		line = strings.TrimRight(line, "\r\n")
		loopEnv := newEnv(env)
		loopEnv.declare(s.Var, stringValue(line))
		c, err := in.execBlock(s.Body, loopEnv, cc)
		if err != nil {
			return ctrl{}, err
		}
		if out, ok := handleLoopCtrl(c); !ok {
			return out, nil
		}
		if readErr != nil {
			return ctrl{}, nil
		}
	}
}

// handleLoopCtrl - единая точка, где цикл (WhileStmt/ForStmt/ForEachStmt/
// CStyleForStmt, оба варианта - коллекция и построчное чтение файла)
// решает, что делать с ctrl, полученным от одной итерации своего тела.
// ok=false - вызывающий цикл обязан немедленно вернуть out как СВОЙ
// результат (сам он больше не итерирует): настоящее "вернуть" (out - тот же
// ctrlReturn без изменений), "прервать"/"продолжить" ЭТОГО цикла (depth<=1 -
// возвращённый out - ctrl{} - обычное завершение цикла, "прервать"/
// "продолжить" полностью поглощены здесь), либо "прервать N"/"продолжить N"
// , нацеленные на цикл СНАРУЖИ (depth>1) - этот цикл тоже
// обязан остановиться (как при "прервать"), но пробрасывает out дальше с
// depth-1 - на следующем уровне наружу depth снова сравнивается с 1, и так
// далее, пока не дойдёт до нужного цикла.
// ok=true - сигнал не требовал остановки ЭТОГО цикла (ctrlNone или
// ctrlContinue с depth<=1 - "своё" продолжение) - вызывающий код продолжает
// как раньше (у CStyleForStmt - сначала исполнив Post).
func handleLoopCtrl(c ctrl) (out ctrl, ok bool) {
	switch c.kind {
	case ctrlReturn:
		return c, false
	case ctrlBreak:
		if c.depth > 1 {
			return ctrl{kind: ctrlBreak, depth: c.depth - 1}, false
		}
		return ctrl{}, false
	case ctrlContinue:
		if c.depth > 1 {
			return ctrl{kind: ctrlContinue, depth: c.depth - 1}, false
		}
		return ctrl{}, true
	default: // ctrlNone
		return ctrl{}, true
	}
}
