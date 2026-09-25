package lsp

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// hoverInterface - hover на имени интерфейса: сигнатуры его методов и
// doc-комментарий объявления.
func (d *doc) hoverInterface(name string) (string, bool) {
	iface, ok := d.astInterfaces[name]
	if !ok {
		return "", false
	}
	header := "интерфейс " + name
	if len(iface.Extends) > 0 {
		header += " наследует " + strings.Join(iface.Extends, ", ")
	}
	lines := make([]string, len(iface.Methods))
	for i, m := range iface.Methods {
		lines[i] = "    " + interfaceMethodSignature(m)
	}
	body := header + "\n{\n" + strings.Join(lines, "\n") + "\n}"
	if len(lines) == 0 {
		body = header + "\n{\n}"
	}
	return withDoc(codeBlock(body), iface.Doc), true
}

// hoverInterfaceMember - hover на методе после получателя, типизированного
// интерфейсом: сигнатура и doc-комментарий метода, в том числе
// унаследованного от родительского интерфейса.
func (d *doc) hoverInterfaceMember(ifaceName, name string) (string, bool) {
	m, owner, ok := d.findInterfaceMethod(ifaceName, name, map[string]bool{})
	if !ok {
		return "", false
	}
	return withDoc(codeBlock(owner+"."+interfaceMethodSignature(m)), m.Doc), true
}

// findInterfaceMethod - метод name интерфейса ifaceName или его предков
// ("наследует"); seen защищает от цикла наследования в недописанном коде.
func (d *doc) findInterfaceMethod(ifaceName, name string, seen map[string]bool) (ast.InterfaceMethod, string, bool) {
	iface, ok := d.astInterfaces[ifaceName]
	if !ok || seen[ifaceName] {
		return ast.InterfaceMethod{}, "", false
	}
	seen[ifaceName] = true
	for _, m := range iface.Methods {
		if m.Name == name {
			return m, ifaceName, true
		}
	}
	for _, parent := range iface.Extends {
		if m, owner, ok := d.findInterfaceMethod(parent, name, seen); ok {
			return m, owner, true
		}
	}
	return ast.InterfaceMethod{}, "", false
}

// interfaceMethodSignature - "имя(п: Тип, ...): Результат" метода интерфейса
// в записи исходника.
func interfaceMethodSignature(m ast.InterfaceMethod) string {
	params := make([]string, len(m.Params))
	for i, p := range m.Params {
		params[i] = fmt.Sprintf("%s: %s", p.Name, p.Type)
	}
	sig := fmt.Sprintf("%s(%s)", m.Name, strings.Join(params, ", "))
	if m.ReturnType != "" {
		sig += ": " + m.ReturnType
	}
	return sig
}
