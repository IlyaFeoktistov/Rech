package morph

// VerbForms - ключевые формы глагола, задаваемые явно при регистрации (и
// для системного словаря, и для пользовательских методов). В отличие от
// существительных, спряжение не выводится автоматическим классификатором в
// Фазе A - см. docs/MORPHOLOGY_DESIGN.md, "Глаголы: явно заданные формы, без
// угадывания". PresentOrFuture3Sg - форма 3-го лица единственного числа:
// для несовершенного вида это настоящее время ("создаёт"), для
// совершенного - будущее ("создаст"), сам вид в Фазе A не хранится
// (появится в шаге 6.2).
type VerbForms struct {
	Infinitive         string
	PresentOrFuture3Sg string
	PastM              string
	PastF              string
	PastN              string
	PastPl             string
	Gerund             string // деепричастие - используется с шага 6.5
	PastParticiple     string // причастие действ. прош. времени - шаг 6.4
}

// VerbFormKind - какой конкретно слот VerbForms соответствует словоформе,
// найденной при обратном поиске (Dictionary.Analyze).
type VerbFormKind int

const (
	FormInfinitive VerbFormKind = iota
	FormPresentOrFuture3Sg
	FormPastM
	FormPastF
	FormPastN
	FormPastPl
	FormGerund
	FormPastParticiple
)

func (k VerbFormKind) String() string {
	switch k {
	case FormInfinitive:
		return "инфинитив"
	case FormPresentOrFuture3Sg:
		return "3-е лицо ед.ч. наст./буд. времени"
	case FormPastM:
		return "прошедшее время, муж. род"
	case FormPastF:
		return "прошедшее время, жен. род"
	case FormPastN:
		return "прошедшее время, сред. род"
	case FormPastPl:
		return "прошедшее время, мн. число"
	case FormGerund:
		return "деепричастие"
	case FormPastParticiple:
		return "причастие"
	default:
		return "неизвестная форма глагола"
	}
}

// VerbInfo - словарная запись глагола.
type VerbInfo struct {
	Forms VerbForms
}
