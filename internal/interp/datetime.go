package interp

import (
	"fmt"
	"strconv"
	"time"
	_ "time/tzdata" // встраивает полную базу IANA tzdata в сам бинарник rech - см. package doc localizedTimeToDate

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

// newDateTimeValue - собирает Value сущности "ДатаВремя" из шести
// календарных полей - общая точка для parseDateValue/timeToDateInZoneValue,
// вместо повторения одной и той же карты полей в обеих.
func newDateTimeValue(year, month, day, hour, min, sec int64) Value {
	fields := map[string]Value{
		"год": intValue(year), "месяц": intValue(month), "день": intValue(day),
		"час": intValue(hour), "минута": intValue(min), "секунда": intValue(sec),
	}
	return Value{Type: ir.EntityType("ДатаВремя"), Obj: &entityObj{typeName: "ДатаВремя", fields: fields}}
}

// nullableDateTimeType/nullableIntType - "?ДатаВремя"/"?Целое" - общие
// константы для всех nullable-результатов этого файла (parseDateValue/
// timeToDateInZoneValue/dateInZoneToTimeValue).
var nullableDateTimeType = ir.NullableType(ir.EntityType("ДатаВремя"))
var nullableIntType = ir.NullableType(ir.TypeInt)

// parseDateValue - "разобрать_дату(строка): ?ДатаВремя" - строгий разбор
// формата "ГГГГ-ММ-ДД ЧЧ:ММ:СС", обратного к dateToString - Ничто на любое
// отклонение от формата (длина/разделители/нечисловые символы/поле вне
// диапазона), не паника - тем же принципом, что и у остальных builtin'ов,
// возвращающих nullable при "плохом" входе (получить_переменную_окружения,
// получить_поле_по_тегу).
func parseDateValue(s string) Value {
	if len(s) != 19 || s[4] != '-' || s[7] != '-' || s[10] != ' ' || s[13] != ':' || s[16] != ':' {
		return Value{Type: nullableDateTimeType}
	}
	year, err1 := strconv.ParseInt(s[0:4], 10, 64)
	month, err2 := strconv.ParseInt(s[5:7], 10, 64)
	day, err3 := strconv.ParseInt(s[8:10], 10, 64)
	hour, err4 := strconv.ParseInt(s[11:13], 10, 64)
	min, err5 := strconv.ParseInt(s[14:16], 10, 64)
	sec, err6 := strconv.ParseInt(s[17:19], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil {
		return Value{Type: nullableDateTimeType}
	}
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || min > 59 || sec > 59 {
		return Value{Type: nullableDateTimeType}
	}
	d := newDateTimeValue(year, month, day, hour, min, sec)
	return Value{Type: nullableDateTimeType, Nullable: &d}
}

// timeToDateInZoneValue - "время_как_дата_в_поясе(метка, пояс): ?ДатаВремя" -
// в отличие от timeToDate (всегда UTC), раскладывает unix-метку в
// календарные поля МЕСТНОГО времени указанного пояса (имя базы IANA,
// "Europe/Moscow") - Ничто, если имя пояса не распознано. time.LoadLocation
// сам решает переходы на летнее/зимнее время по полной исторической базе
// (см. blank-импорт "time/tzdata" выше) - в отличие от cbackend
// (internal/backend/cbackend/tzdata.go), здесь НЕ нужен собственный
// TZif-читатель: то же самое уже есть в стандартной библиотеке Go.
func timeToDateInZoneValue(ts int64, zone string) Value {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Value{Type: nullableDateTimeType}
	}
	t := time.Unix(ts, 0).In(loc)
	y, mo, day := t.Date()
	hour, min, sec := t.Clock()
	d := newDateTimeValue(int64(y), int64(mo), int64(day), int64(hour), int64(min), int64(sec))
	return Value{Type: nullableDateTimeType, Nullable: &d}
}

// dateInZoneToTimeValue - "дата_в_поясе_как_время(дата, пояс): ?Целое" -
// обратное: календарные поля d ИНТЕРПРЕТИРУЮТСЯ как настенное время в
// указанном поясе, результат - unix-метка (UTC). Ничто, если имя пояса не
// распознано. time.Date сам разрешает неоднозначность/пропуск часа
// перехода летнее/зимнее по своим внутренним правилам - см. документацию
// time.Date, тот же уровень точности, что и у cbackend-читателя (одна
// итерация уточнения смещения, см. package doc rech_date_to_time_tz).
func dateInZoneToTimeValue(d Value, zone string) Value {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Value{Type: nullableIntType}
	}
	f := d.Obj.fields
	t := time.Date(int(f["год"].I), time.Month(f["месяц"].I), int(f["день"].I),
		int(f["час"].I), int(f["минута"].I), int(f["секунда"].I), 0, loc)
	ts := intValue(t.Unix())
	return Value{Type: nullableIntType, Nullable: &ts}
}

// dayOfWeekValue - "день_недели(д): Целое" (0 = воскресенье .. 6 = суббота,
// см. package doc internal/backend/cbackend/datetime_human.go) - только
// календарная дата важна, не время суток - тот же daysFromCivil/weekday-
// расчёт, что и у cbackend rech_day_of_week (не дублирует его алгоритм
// заново - обе стороны считают ОДНО и то же: 1970-01-01 - четверг).
func dayOfWeekValue(d Value) Value {
	f := d.Obj.fields
	days := daysFromCivil(f["год"].I, f["месяц"].I, f["день"].I)
	w := (days + 4) % 7
	if w < 0 {
		w += 7
	}
	return intValue(w)
}

// ruPlural - русское склонение числительного (1/2-4/5+, с исключением
// 11-14) - общая точка для humanDateValue (единственный вызывающий пока,
// но не специфична для дат - осознанно не привязана к unit-конкретике).
func ruPlural(n int64, one, few, many string) string {
	n100 := n % 100
	if n100 >= 11 && n100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

// humanDateValue - "дату_в_читаемом_виде(д): Строка" (и "д.в_читаемом_виде()") -
// относительное время в стиле Carbon'овского diffForHumans(), см. package
// doc internal/backend/cbackend/datetime_human.go.
func humanDateValue(d Value) Value {
	ts := dateToTime(d)
	now := nowUnix()
	diff := now - ts
	future := diff < 0
	if diff < 0 {
		diff = -diff
	}
	if diff < 5 {
		return stringValue("только что")
	}
	var n int64
	var unit string
	switch {
	case diff < 60:
		n = diff
		unit = ruPlural(n, "секунда", "секунды", "секунд")
	case diff < 3600:
		n = diff / 60
		unit = ruPlural(n, "минута", "минуты", "минут")
	case diff < 86400:
		n = diff / 3600
		unit = ruPlural(n, "час", "часа", "часов")
	default:
		n = diff / 86400
		unit = ruPlural(n, "день", "дня", "дней")
	}
	if future {
		return stringValue(fmt.Sprintf("через %d %s", n, unit))
	}
	return stringValue(fmt.Sprintf("%d %s назад", n, unit))
}
