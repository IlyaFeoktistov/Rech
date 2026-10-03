package interp

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// numeric/DECIMAL ([]byte от lib/pq и go-sql-driver/mysql) - Десятичное с
// масштабом из БД; прочий текст - Строка.
func TestDBBoxColumnValueNumericIsDecimal(t *testing.T) {
	v := dbBoxColumnValue([]byte("129.90"), true)
	if v.Boxed.Type != ir.TypeDecimal || v.Boxed.Dec.String() != "129.90" {
		t.Fatalf("numeric: получено %v", *v.Boxed)
	}
	if v := dbBoxColumnValue([]byte("129.90"), false); v.Boxed.Type != ir.TypeString {
		t.Fatalf("текст: получено %v", *v.Boxed)
	}
	if v := dbBoxColumnValue(0.5, true); v.Boxed.Type != ir.TypeFloat {
		t.Fatalf("double остаётся Действительное: получено %v", *v.Boxed)
	}
}

func TestDBNumericValueNaNPanicsBadCast(t *testing.T) {
	defer func() {
		r, ok := recover().(*runtimeError)
		if !ok || r.kind != rtmsg.PanicBadCast || !strings.Contains(r.msg, rtmsg.DBNumericNotNumber+"NaN") {
			t.Fatalf("ожидалась паника НеверноеПриведение про NaN, получено %#v", r)
		}
	}()
	dbNumericValue("NaN")
}
