package cbackend

import "testing"

// TestMangleMethodHandlesOperatorSyntax - синтетическое имя перегрузки
// оператора (ast.OperatorMethodName, "оператор+"/"оператор=="/...) обязано
// транслитерироваться в валидный C-идентификатор (никаких "+", "-", "*",
// "/", "%", "=", "!", "<", ">" внутри) и различаться для разных символов -
// тот же принцип, что TestMangleMethodHandlesOverloadSyntax для обычной
// перегрузки (overload_test.go).
func TestMangleMethodHandlesOperatorSyntax(t *testing.T) {
	invalid := map[rune]bool{'+': true, '-': true, '*': true, '/': true, '%': true, '=': true, '!': true, '<': true, '>': true, '(': true, ')': true, ',': true}
	cases := []string{
		"оператор+", "оператор-", "оператор*", "оператор/", "оператор%",
		"оператор==", "оператор!=", "оператор<", "оператор>", "оператор<=", "оператор>=",
	}
	seen := map[string]string{}
	for _, name := range cases {
		mangled := mangleMethod("Вектор", name)
		for _, r := range mangled {
			if invalid[r] {
				t.Fatalf("mangleMethod(Вектор, %q) = %q содержит недопустимый для C-идентификатора символ %q", name, mangled, r)
			}
		}
		if other, exists := seen[mangled]; exists {
			t.Fatalf("mangleMethod(Вектор, %q) и mangleMethod(Вектор, %q) дали одинаковый C-идентификатор %q - операторы неразличимы", name, other, mangled)
		}
		seen[mangled] = name
	}
}
