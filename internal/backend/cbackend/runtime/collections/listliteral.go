package collections

import "fmt"

// ListLiteralCode - "построить из литерала" для одной инстанциации
// Список<T> (см. package doc ir.ListLiteralExpr) - GC_MALLOC, тем же
// принципом, что и listCollectionCCode ("добавить" растит this_->data
// через GC_REALLOC, см. её package doc) - этот буфер дальше растёт тем же
// GC_REALLOC при последующих "добавить", поэтому должен быть выделен через
// Boehm GC с самого начала, не голым malloc. n==0 (пустой литерал с
// известным по аннотации типу) вызывается отдельно как "fn(0, NULL)" на
// месте (см. generateIRExpr) - цикл заполнения здесь безопасен и для этого
// случая (0 итераций), values не разыменовывается. structC/elemC/ctorName -
// уже вычисленные вызывающей стороной (mangleType/mangleCtor - внутренняя,
// неэкспортированная инфраструктура cbackend, этот пакет от неё не
// зависит).
func ListLiteralCode(structC, elemC, ctorName string) string {
	fn := structC + "_from_literal"
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

/* Склейка частей литерала со спредом ("[a, ...b, c]"): каждая часть -
   готовый список, результат - новый список из всех их элементов по
   порядку. */
static %[1]s *%[1]s_from_parts(long long n, %[1]s **parts) {
    long long total = 0;
    for (long long i = 0; i < n; i++) {
        total += parts[i]->len;
    }
    %[3]s *data = total > 0 ? GC_MALLOC(sizeof(%[3]s) * (size_t)total) : NULL;
    long long k = 0;
    for (long long i = 0; i < n; i++) {
        for (long long j = 0; j < parts[i]->len; j++) {
            data[k++] = parts[i]->data[j];
        }
    }
    %[1]s *r = %[4]s();
    r->data = data;
    r->len = total;
    r->cap = total;
    return r;
}

`, structC, fn, elemC, ctorName)
}
