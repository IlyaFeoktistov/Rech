package interp

import (
	"bufio"
	"os"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// execStmt исполняет одну инструкцию - см. execBlock (interp.go) про то, как
// ctrlReturn прерывает выполнение вверх по стеку блоков/вызову.
func (in *Interp) execStmt(s ir.Stmt, env *Env, cc *callCtx) (ctrl, error) {
	switch s := s.(type) {
	case *ir.PrintStmt:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		in.printValue(v)
		return ctrl{}, nil

	case *ir.VarDecl:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		env.declare(s.Name, cloneValue(v))
		return ctrl{}, nil

	case *ir.Assign:
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		env.set(s.Name, cloneValue(v))
		return ctrl{}, nil

	case *ir.FieldAssign:
		recv, err := in.evalExpr(s.Receiver, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		recv.Obj.fields[s.Field] = cloneValue(v)
		return ctrl{}, nil

	case *ir.BecomeStmt:
		// Шаг 4.1, упрощённая версия: "становится" структурно проверено
		// семантическим анализом и не несёт значения во время исполнения -
		// как и в cbackend (не эмитит код), здесь просто ничего не делает.
		return ctrl{}, nil

	case *ir.AssertStmt:
		cond, err := in.evalExpr(s.Cond, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if !cond.B {
			panicRuntime("%s", rtmsg.AssertFailed(s.Line))
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
			if c.kind == ctrlReturn {
				return c, nil
			}
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
		for i := from.I; i <= to.I; i++ {
			loopEnv := newEnv(env)
			loopEnv.declare(s.Var, intValue(i))
			c, err := in.execBlock(s.Body, loopEnv, cc)
			if err != nil {
				return ctrl{}, err
			}
			if c.kind == ctrlReturn {
				return c, nil
			}
		}
		return ctrl{}, nil

	case *ir.ForEachStmt:
		collV, err := in.evalExpr(s.Collection, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if collV.Type.Kind == ir.KindFile {
			return in.execForEachFileLine(collV.File, s, env, cc)
		}
		elems := collV.Obj.coll.elems
		for _, elem := range elems {
			loopEnv := newEnv(env)
			loopEnv.declare(s.Var, cloneValue(elem))
			c, err := in.execBlock(s.Body, loopEnv, cc)
			if err != nil {
				return ctrl{}, err
			}
			if c.kind == ctrlReturn {
				return c, nil
			}
		}
		return ctrl{}, nil

	case *ir.ReturnStmt:
		if s.Value == nil {
			return ctrl{kind: ctrlReturn}, nil
		}
		v, err := in.evalExpr(s.Value, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		return ctrl{kind: ctrlReturn, value: cloneValue(v)}, nil

	case *ir.ExprStmt:
		var e ir.Expr = s.Call
		switch {
		case s.MethodCall != nil:
			e = s.MethodCall
		case s.IncDec != nil:
			e = s.IncDec
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
	reader := bufio.NewReader(f)
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
		if c.kind == ctrlReturn {
			return c, nil
		}
		if readErr != nil {
			return ctrl{}, nil
		}
	}
}
