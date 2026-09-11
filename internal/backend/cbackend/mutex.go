// Синхронизация между потоками - "Блокировка" (голый примитив поверх
// нативного мьютекса ОС - pthread_mutex_t на POSIX, CRITICAL_SECTION на
// Windows, тот же принцип кроссплатформенности, что и у потоков самих по
// себе, concurrency.go - без встроенной проверки гонок, решение
// пользователя явно, "Синхронизация между потоками").
//
// "Блокировка" - не обычная сущность (нет ast.EntityDecl, ent.Methods/
// ent.Ctor пусты, см. package doc ir.Entity.IsMutex) - её C-структура,
// конструктор и оба метода (захватить/освободить) целиком синтезированы
// здесь, а не генератором обычных методов/полей (generateIREntityStruct/
// generateIREntityCtor только распознают IsMutex и отдают этот текст, не
// строят его из Fields/Methods, которых у неё попросту нет).
package cbackend

import "fmt"

const mutexEntityName = "Блокировка"

// mutexStructTypedef - C-структура "Блокировка": единственный член -
// нативный примитив ОС, без "vt" (никогда не участвует в наследовании/
// интерфейсах).
func mutexStructTypedef() string {
	return fmt.Sprintf(
		"typedef struct {\n#ifdef _WIN32\n    CRITICAL_SECTION native;\n#else\n    pthread_mutex_t native;\n#endif\n} %s;\n\n",
		mangleType(mutexEntityName),
	)
}

// mutexPrototypes - прототипы конструктора и обоих методов "Блокировки" -
// эмитятся в общий "protos" builder (ir_generator.go), независимо от того,
// в какой момент выполнения программы её ir.Entity добавлен в
// prog.Entities (см. package doc internal/semantic/analyzer.go, buildIR,
// про то, почему это может случиться поздно, уже после кода, который её
// использует).
func mutexPrototypes() string {
	cType := mangleType(mutexEntityName)
	return fmt.Sprintf(
		"static %[1]s *%[2]s(void);\nstatic void %[3]s(%[1]s *this_);\nstatic void %[4]s(%[1]s *this_);\n",
		cType, mangleCtor(mutexEntityName), mangleMethod(mutexEntityName, "захватить"), mangleMethod(mutexEntityName, "освободить"),
	)
}

// mutexRuntimeCode - конструктор ("новая Блокировка()") и оба метода
// ("захватить"/"освободить") одним куском - эмитится безусловно, ровно
// один раз, только если ctx.usesMutex (см. GenerateFromIR). Позиция в файле
// (тот же builder, что и generateIREntityCtor обычных сущностей) - раньше
// тел функций/методов программы, которые могут её вызывать.
func mutexRuntimeCode() string {
	cType := mangleType(mutexEntityName)
	ctor := mangleCtor(mutexEntityName)
	lock := mangleMethod(mutexEntityName, "захватить")
	unlock := mangleMethod(mutexEntityName, "освободить")
	return fmt.Sprintf(`static %[1]s *%[2]s(void) {
    %[1]s *p = GC_MALLOC(sizeof(%[1]s));
#ifdef _WIN32
    InitializeCriticalSection(&p->native);
#else
    pthread_mutex_init(&p->native, NULL);
#endif
    return p;
}

static void %[3]s(%[1]s *this_) {
#ifdef _WIN32
    EnterCriticalSection(&this_->native);
#else
    pthread_mutex_lock(&this_->native);
#endif
}

static void %[4]s(%[1]s *this_) {
#ifdef _WIN32
    LeaveCriticalSection(&this_->native);
#else
    pthread_mutex_unlock(&this_->native);
#endif
}

`, cType, ctor, lock, unlock)
}
