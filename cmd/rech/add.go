// "rech add" - добавляет новую внешнюю зависимость в файл проекта
// (.rechproj, секция "зависимости:") и сразу скачивает её - см.
// internal/pkgfetch и cmd/rech/deps.go (resolveDeps) для самого скачивания,
// этот файл - только текстовое редактирование .rechproj.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/pkgfetch"
	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// addDependency реализует "rech add <имя> <url> <версия>": дописывает
// "  имя: url версия" в секцию "зависимости:" файла проекта (создавая саму
// секцию, если её ещё нет), затем скачивает ТОЛЬКО что добавленную
// зависимость (заодно и все остальные уже объявленные - resolveDeps не
// умеет частично, но это тот же путь, что и "rech install") - пользователь
// сразу видит результат скачивания и коммит, на котором она зафиксировалась
// в .rechlock, без отдельного следующего "rech install".
func addDependency(name, url, version string) error {
	if err := validateDepName(name); err != nil {
		return err
	}
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("URL зависимости не может быть пустым")
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("версия зависимости (тег/ветка/коммит) не может быть пустой - \"rech add\" не угадывает её сама")
	}

	projPath, err := project.Find(".")
	if err != nil {
		return err
	}
	if projPath == "" {
		return fmt.Errorf("в текущем каталоге не найден файл проекта (*.rechproj) - сначала \"rech init\"")
	}

	pf, err := project.Load(projPath)
	if err != nil {
		return err
	}
	if _, exists := pf.Зависимости[name]; exists {
		return fmt.Errorf("зависимость %q уже объявлена в %s - отредактируйте файл вручную, если нужно изменить URL/версию", name, projPath)
	}

	data, err := os.ReadFile(projPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(projPath, []byte(insertDependencyLine(string(data), name, url, version)), 0o644); err != nil {
		return fmt.Errorf("не удалось записать %s: %w", projPath, err)
	}
	fmt.Printf("добавлено в %s: %s (%s %s)\n", projPath, name, url, version)

	pf, err = project.Load(projPath)
	if err != nil {
		return fmt.Errorf("%s изменён, но не разбирается: %w - проверьте файл вручную", projPath, err)
	}
	_, scopes, libs, _, err := resolveDeps(pf)
	if err != nil {
		return err
	}
	total := 0
	for _, s := range scopes {
		total += len(s)
	}
	fmt.Printf("готово: %d завис. (lock-файл %s)\n", total, filepath.Join(pf.Dir, pkgfetch.LockFileName))
	if len(libs) > 0 {
		fmt.Println("библиотеки для линковки (из зависимостей):", libs)
	}
	return nil
}

// validateDepName проверяет короткое локальное имя зависимости - как оно
// появится и в .rechproj (ключ секции "зависимости:"), и в исходниках
// потребителя ("использовать X из \"имя\""): непустое, без пробелов/
// двоеточий/"#" (те же символы, которые ломают построчный разбор
// internal/project.Load).
func validateDepName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("имя зависимости не может быть пустым")
	}
	if strings.ContainsAny(name, " \t\n:#") {
		return fmt.Errorf("имя зависимости %q не может содержать пробелы, \":\" или \"#\"", name)
	}
	return nil
}

// insertDependencyLine дописывает "  имя: url версия" в секцию
// "зависимости:" текста файла проекта text - последней строкой секции
// (после уже объявленных зависимостей, если есть), или создаёт саму секцию
// в конце файла, если активной секции ещё нет (закомментированная подсказка
// project.Template, "# зависимости:", - не топ-уровневая секция, "#" делает
// её просто комментарием, TrimSpace даёт "# зависимости:" != "зависимости:").
// Единственное место, которое пишет .rechproj программно - точечное
// текстовое редактирование одной секции, а не сериализация всего File,
// чтобы не потерять комментарии/форматирование остальных секций
// (internal/project не хранит исходный текст целиком, только разобранные
// поля).
func insertDependencyLine(text, name, url, version string) string {
	depLine := fmt.Sprintf("  %s: %s %s", name, url, version)
	lines := strings.Split(text, "\n")

	sectionIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		topLevel := trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")
		if topLevel && trimmed == "зависимости:" {
			sectionIdx = i
			break
		}
	}

	if sectionIdx == -1 {
		out := strings.TrimRight(text, "\n")
		if out != "" {
			out += "\n\n"
		}
		return out + "зависимости:\n" + depLine + "\n"
	}

	end := len(lines)
	for i := sectionIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		topLevel := trimmed != "" && !strings.HasPrefix(lines[i], " ") && !strings.HasPrefix(lines[i], "\t")
		if topLevel {
			end = i
			break
		}
	}
	for end > sectionIdx+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:end]...)
	out = append(out, depLine)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}
