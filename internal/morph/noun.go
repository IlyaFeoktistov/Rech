package morph

import (
	"fmt"
	"strings"
)

// DeclensionClass - класс склонения существительного. Фаза A (см.
// docs/MORPHOLOGY.md) покрывает шесть самых частых для словаря программы
// классов; менее частые (женский род на "-ь", "-мя"-исключения,
// несклоняемые слова) - Фаза B.
type DeclensionClass int

const (
	ClassFirstHard       DeclensionClass = iota // документ (муж., твёрдая, согласная)
	ClassFirstSoft                              // пользователь (муж., мягкая, "-ь")
	ClassSecondHard                             // система (жен., твёрдая, "-а")
	ClassSecondSoft                             // компания (жен., мягкая, "-ия")
	ClassNeuterHard                             // окно (сред., твёрдая, "-о")
	ClassNeuterSoft                             // сообщение (сред., мягкая, "-ие")
	ClassSecondSoftPlain                        // земля (жен., мягкая, голое "-я", без предшествующей "и")

	// ClassIrregular - слово зарегистрировано по готовой парадигме
	// (nounFullExceptions в exceptions.go), не generic-классом склонения -
	// у него есть чередование основы, не выводимое ни одним правилом ниже
	// (см. docs/MORPHOLOGY.md, "Полностью готовая парадигма без генератора").
	// classTable для него не заполнена - BuildNounParadigm с этим классом не
	// вызывается.
	ClassIrregular
)

// endings - окончания одного класса склонения: [единственное, множественное]
// для каждого падежа, кроме винительного - он не хранится отдельно, а
// вычисляется по одушевлённости (см. BuildNounParadigm): у одушевлённых
// совпадает с родительным, у неодушевлённых - с именительным.
type endings struct {
	nom, gen, dat, instr, prep [2]string
}

type classInfo struct {
	strip   string // суффикс, который нужно отбросить от им.п. ед.ч., чтобы получить основу
	gender  Gender
	endings endings

	// accSg - винительный падеж ед.ч., если он не сводится ни к
	// именительному, ни к родительному (это истинно для классов женского
	// рода на "-а"/"-ия": "система" -> "системУ", "компания" -> "компаниЮ" -
	// у них своё окончание вин.п. ед.ч. независимо от одушевлённости).
	// Пусто - винительный ед.ч. вычисляется по одушевлённости, как обычно
	// (совпадает с родительным у одушевлённых, с именительным у
	// неодушевлённых) - это верно для муж./сред. рода. Во множественном
	// числе правило по одушевлённости работает одинаково для всех родов,
	// accSg на него не влияет.
	accSg string
}

var classTable = map[DeclensionClass]classInfo{
	ClassFirstHard: {
		strip:  "",
		gender: Masculine,
		endings: endings{
			nom:   [2]string{"", "ы"},
			gen:   [2]string{"а", "ов"},
			dat:   [2]string{"у", "ам"},
			instr: [2]string{"ом", "ами"},
			prep:  [2]string{"е", "ах"},
		},
	},
	ClassFirstSoft: {
		strip:  "ь",
		gender: Masculine,
		endings: endings{
			nom:   [2]string{"ь", "и"},
			gen:   [2]string{"я", "ей"},
			dat:   [2]string{"ю", "ям"},
			instr: [2]string{"ем", "ями"},
			prep:  [2]string{"е", "ях"},
		},
	},
	ClassSecondHard: {
		strip:  "а",
		gender: Feminine,
		endings: endings{
			nom:   [2]string{"а", "ы"},
			gen:   [2]string{"ы", ""},
			dat:   [2]string{"е", "ам"},
			instr: [2]string{"ой", "ами"},
			prep:  [2]string{"е", "ах"},
		},
		accSg: "у",
	},
	ClassSecondSoft: {
		strip:  "ия",
		gender: Feminine,
		endings: endings{
			nom:   [2]string{"ия", "ии"},
			gen:   [2]string{"ии", "ий"},
			dat:   [2]string{"ии", "иям"},
			instr: [2]string{"ией", "иями"},
			prep:  [2]string{"ии", "иях"},
		},
		accSg: "ию",
	},
	ClassNeuterHard: {
		strip:  "о",
		gender: Neuter,
		endings: endings{
			nom:   [2]string{"о", "а"},
			gen:   [2]string{"а", ""},
			dat:   [2]string{"у", "ам"},
			instr: [2]string{"ом", "ами"},
			prep:  [2]string{"е", "ах"},
		},
	},
	ClassNeuterSoft: {
		strip:  "ие",
		gender: Neuter,
		endings: endings{
			nom:   [2]string{"ие", "ия"},
			gen:   [2]string{"ия", "ий"},
			dat:   [2]string{"ию", "иям"},
			instr: [2]string{"ием", "иями"},
			prep:  [2]string{"ии", "иях"},
		},
	},
	// ClassSecondSoftPlain - "земля"/"неделя"/"тётя": женский род, голое "-я"
	// (не путать с ClassSecondSoft - "-ия", "компания" - у него своя,
	// отличная парадигма, начинающаяся сразу с "и" во всех косвенных
	// падежах). Как и у ClassSecondHard, родительный падеж мн.ч. - без
	// добавленного окончания (bare-стем): верно для основ без беглой гласной
	// ("недель" из "неделя"), но не для тех, кому беглая гласная нужна
	// ("земля" -> "земель") - беглые гласные в целом вне Фазы A, тот же
	// общий недостаток, что и у ClassSecondHard ("сестра" -> "сестёр").
	ClassSecondSoftPlain: {
		strip:  "я",
		gender: Feminine,
		endings: endings{
			nom:   [2]string{"я", "и"},
			gen:   [2]string{"и", ""},
			dat:   [2]string{"е", "ям"},
			instr: [2]string{"ей", "ями"},
			prep:  [2]string{"е", "ях"},
		},
		accSg: "ю",
	},
}

// GenderOf возвращает род, закреплённый за классом склонения.
func GenderOf(class DeclensionClass) Gender {
	return classTable[class].gender
}

// ClassifyByEnding выводит класс склонения по окончанию именительного
// падежа единственного числа. Возвращает ok=false, если окончание не
// определяет класс однозначно в Фазе A - это в первую очередь слова на
// "-ь" (могут быть и мужского, и женского рода) и на "-й"/"голое -е"
// (отдельные подтипы, отложенные до Фазы B). В этом случае лемму нужно
// зарегистрировать через Dictionary.RegisterNounExplicit или добавить в
// словарь исключений (exceptions.go) - никогда не угадывается "на удачу".
//
// "-я" (голое, проверяется после "-ия" - иначе "-ия" никогда бы не
// сработала, т.к. тоже оканчивается на "-я") - женский род,
// ClassSecondSoftPlain ("земля"); default ниже - это именно "согласная
// (иначе)", а не "что угодно не подошедшее выше", поэтому гласному "-я"
// нужен свой явный case. Разносклоняемые "-мя"-слова (имя, время, пламя,
// ...) сюда не попадают - у них не только другой род (средний), но и вся
// парадигма нерегулярна ("имя"/"имени"/"именем", не "имя"/"и"/"ей" по этому
// классу) - они регистрируются раньше, через nounFullExceptions.
func ClassifyByEnding(nomSg string) (DeclensionClass, bool) {
	switch {
	case strings.HasSuffix(nomSg, "ие"):
		return ClassNeuterSoft, true
	case strings.HasSuffix(nomSg, "ия"):
		return ClassSecondSoft, true
	case strings.HasSuffix(nomSg, "я"):
		return ClassSecondSoftPlain, true
	case strings.HasSuffix(nomSg, "о"):
		return ClassNeuterHard, true
	case strings.HasSuffix(nomSg, "а"):
		return ClassSecondHard, true
	case strings.HasSuffix(nomSg, "ь"), strings.HasSuffix(nomSg, "й"), strings.HasSuffix(nomSg, "е"):
		return 0, false
	case strings.HasSuffix(nomSg, "ы"), strings.HasSuffix(nomSg, "и"):
		// "-ы"/"-и" - обычные окончания множественного числа
		// (маршрут -> маршруты, сессия -> сессии), не именительного падежа
		// единственного числа ни одного класса склонения - default ниже
		// рассчитан именно на "согласная" (см. doc выше), НЕ "что угодно
		// не подошедшее выше", поэтому здесь тоже нужен явный отказ, а не
		// ошибочное падение в ClassFirstHard (тот класс - твёрдая согласная
		// основа, "-ы"/"-и" у него не бывает в единственном числе вовсе).
		// Число (обычно множественное) для этого случая решает
		// NounAgreement (hasPluralEnding), не этот классификатор - он
		// целиком про класс склонения ЕДИНСТВЕННОГО числа, тут только
		// отказ от угадывания класса.
		return 0, false
	default:
		return ClassFirstHard, true
	}
}

func stemOf(nomSg string, class DeclensionClass) (string, error) {
	info, ok := classTable[class]
	if !ok {
		return "", fmt.Errorf("неизвестный класс склонения %v", class)
	}
	if !strings.HasSuffix(nomSg, info.strip) {
		return "", fmt.Errorf("%q не оканчивается на %q, ожидаемое для этого класса склонения", nomSg, info.strip)
	}
	return strings.TrimSuffix(nomSg, info.strip), nil
}

// BuildNounParadigm строит полную парадигму (падеж x число) для nomSg -
// формы именительного падежа единственного числа.
func BuildNounParadigm(nomSg string, class DeclensionClass, animacy Animacy) (Paradigm, error) {
	info, ok := classTable[class]
	if !ok {
		return nil, fmt.Errorf("неизвестный класс склонения %v", class)
	}
	stem, err := stemOf(nomSg, class)
	if err != nil {
		return nil, err
	}

	p := Paradigm{}
	fill := func(c Case, e [2]string) {
		p[Tag{c, Singular}] = stem + e[0]
		p[Tag{c, Plural}] = stem + e[1]
	}
	fill(Nominative, info.endings.nom)
	fill(Genitive, info.endings.gen)
	fill(Dative, info.endings.dat)
	fill(Instrumental, info.endings.instr)
	fill(Prepositional, info.endings.prep)

	accSg := info.accSg
	if accSg != "" {
		accSg = stem + accSg
	}
	fillAccusative(p, animacy, accSg)
	return p, nil
}

// fillAccusative достраивает винительный падеж (ед. и мн. число) в p по
// одушевлённости - общее правило и для генератора (BuildNounParadigm), и
// для готовых парадигм-исключений (nounFullExceptions, registerNounWithParadigm):
// у одушевлённых совпадает с родительным, у неодушевлённых - с
// именительным. accSg - готовая форма вин.п. ед.ч., если она не сводится ни
// к одному из двух (женский род на "-а"/"-ия" независимо от одушевлённости,
// classInfo.accSg) - пусто, если нужно применить обычное правило.
func fillAccusative(p Paradigm, animacy Animacy, accSg string) {
	if accSg != "" {
		p[Tag{Accusative, Singular}] = accSg
	} else if animacy == Animate {
		p[Tag{Accusative, Singular}] = p[Tag{Genitive, Singular}]
	} else {
		p[Tag{Accusative, Singular}] = p[Tag{Nominative, Singular}]
	}

	// Во множественном числе винительный падеж определяется одушевлённостью
	// одинаково для всех родов - accSg (особенность только женского рода в
	// единственном числе) здесь не действует.
	if animacy == Animate {
		p[Tag{Accusative, Plural}] = p[Tag{Genitive, Plural}]
	} else {
		p[Tag{Accusative, Plural}] = p[Tag{Nominative, Plural}]
	}
}
