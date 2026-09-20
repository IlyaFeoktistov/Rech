package semantic

// levenshtein - редакционное расстояние между a и b (число вставок/
// удалений/замен одной руны) - используется только для подсказки
// "возможно, имелось в виду %q" у необъявленных имён (closestName ниже),
// не для чего-либо, влияющего на корректность анализа - реализация
// сознательно простая (полная O(len(a)*len(b)) матрица, без ранних
// отсечений по Ukkonen/битовым векторам), имена программ - короткие
// идентификаторы, не абзацы текста.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			cur[j] = min
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

// closestName возвращает имя из candidates, ближайшее к name по
// levenshtein, если расстояние до него не больше maxDist и он единственный
// на этом минимальном расстоянии (двусмысленная подсказка - "может, X или
// Y" - хуже отсутствия подсказки вообще, не печатается). "", false - нет
// подходящего кандидата.
func closestName(name string, candidates []string, maxDist int) (string, bool) {
	best := ""
	bestDist := maxDist + 1
	ambiguous := false
	for _, c := range candidates {
		if c == name {
			continue
		}
		d := levenshtein(name, c)
		if d > maxDist {
			continue
		}
		switch {
		case d < bestDist:
			best, bestDist, ambiguous = c, d, false
		case d == bestDist:
			ambiguous = true
		}
	}
	if best == "" || ambiguous {
		return "", false
	}
	return best, true
}

// suggestNameHint - "возможно, имелось в виду %q" для closestName(name,
// candidates, 2), иначе "" (без подсказки - см. package doc closestName).
// 2 - расстояние, ловящее типичные опечатки (одна пропущенная/лишняя/
// переставленная буква) без совпадения случайно похожих, но НЕ связанных
// имён из большой программы.
func suggestNameHint(name string, candidates []string) string {
	if c, ok := closestName(name, candidates, 2); ok {
		return "возможно, имелось в виду " + quoteIdent(c)
	}
	return ""
}

// quoteIdent - %q без необходимости импортировать fmt в местах, вызывающих
// suggestNameHint только ради одной этой подстановки.
func quoteIdent(s string) string {
	return "\"" + s + "\""
}
