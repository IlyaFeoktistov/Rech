package cbackend

import "fmt"

// listLiteralCCode - "построить из литерала" для одной инстанциации
// Список<T> (см. package doc ir.ListLiteralExpr) - GC_MALLOC, тем же
// принципом, что и listCollectionCCode ("добавить" растит this_->data
// через GC_REALLOC, см. её package doc) - этот буфер дальше растёт тем же
// GC_REALLOC при последующих "добавить", поэтому должен быть выделен через
// Boehm GC с самого начала, не голым malloc. n==0 (пустой литерал с
// известным по аннотации типу) вызывается отдельно как "fn(0, NULL)" на
// месте (см. generateIRExpr) - цикл заполнения здесь безопасен и для этого
// случая (0 итераций), values не разыменовывается.
func listLiteralCCode(name, elemC string) string {
	structName := mangleType(name)
	fn := structName + "_from_literal"
	ctor := mangleCtor(name)
	return fmt.Sprintf(`static %[1]s *%[2]s(long long n, %[3]s *values) {
    %[3]s *data = n > 0 ? GC_MALLOC(sizeof(%[3]s) * (size_t)n) : NULL;
    for (long long i = 0; i < n; i++) {
        data[i] = values[i];
    }
    %[1]s *r = %[4]s();
    r->data = data;
    r->len = n;
    r->cap = n;
    return r;
}

`, structName, fn, elemC, ctor)
}
