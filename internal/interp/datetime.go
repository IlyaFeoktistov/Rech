package interp

import (
	"fmt"
	"time"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// nowUnix - "время" - текущий момент как unix-метка (секунды с эпохи
// 1970-01-01 UTC, тем же соглашением, что и у cbackend, rech_time_now).
func nowUnix() int64 {
	return time.Now().Unix()
}

// daysFromCivil/civilFromDays - преобразование "дни с эпохи 1970-01-01" в
// григорианские год/месяц/день и обратно - алгоритм Ховарда Хиннанта
// (http://howardhinnant.github.io/date_algorithms.html, общественное
// достояние), чистая целочисленная арифметика без обращения к системным
// календарным функциям ОС (gmtime_r/timegm/gmtime_s/_mkgmtime - их
// доступность и точное поведение расходится между POSIX/MinGW/musl,
// собственная реализация переносима без единого #ifdef и, что важнее,
// БУКВАЛЬНО ОДНА И ТА ЖЕ на обоих backend'ах - internal/backend/cbackend
// использует порт того же алгоритма на C, byte-for-byte результат
// гарантирован по построению, а не совпадением поведения двух разных
// платформенных библиотек). Проверено 200000 случайных меток времени
// (включая отрицательные, до 1970 года) против time.Unix(...).UTC() из
// стандартной библиотеки Go - см. сопроводительный тест.
func daysFromCivil(y int64, m, d int64) int64 {
	if m <= 2 {
		y--
	}
	var era int64
	if y >= 0 {
		era = y / 400
	} else {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mAdj := int64(9)
	if m > 2 {
		mAdj = -3
	}
	doy := (153*(m+mAdj)+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

func civilFromDays(z int64) (y, m, d int64) {
	z += 719468
	var era int64
	if z >= 0 {
		era = z / 146097
	} else {
		era = (z - 146096) / 146097
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y = yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d = doy - (153*mp+2)/5 + 1
	if mp < 10 {
		m = mp + 3
	} else {
		m = mp - 9
	}
	if m <= 2 {
		y++
	}
	return y, m, d
}

// timeToDate - "время_как_дата" - распаковывает unix-метку (секунды, UTC) в
// сущность "ДатаВремя".
func timeToDate(ts int64) Value {
	days := ts / 86400
	rem := ts % 86400
	if rem < 0 {
		rem += 86400
		days--
	}
	y, mo, d := civilFromDays(days)
	hour, min, sec := rem/3600, (rem/60)%60, rem%60
	fields := map[string]Value{
		"год": intValue(y), "месяц": intValue(mo), "день": intValue(d),
		"час": intValue(hour), "минута": intValue(min), "секунда": intValue(sec),
	}
	return Value{Type: ir.EntityType("ДатаВремя"), Obj: &entityObj{typeName: "ДатаВремя", fields: fields}}
}

// dateToTime - "дата_как_время", обратное timeToDate.
func dateToTime(d Value) int64 {
	f := d.Obj.fields
	days := daysFromCivil(f["год"].I, f["месяц"].I, f["день"].I)
	return days*86400 + f["час"].I*3600 + f["минута"].I*60 + f["секунда"].I
}

// dateToString - "дату_в_строку" - "ГГГГ-ММ-ДД ЧЧ:ММ:СС", тот же формат,
// что и cbackend (rech_date_to_string).
func dateToString(d Value) string {
	f := d.Obj.fields
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d",
		f["год"].I, f["месяц"].I, f["день"].I, f["час"].I, f["минута"].I, f["секунда"].I)
}
