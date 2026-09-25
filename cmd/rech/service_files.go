package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend"
)

// Файлы установки службы ("сборка.тип: служба"), которые "rech build"
// кладёт рядом с собранной программой. Саму систему rech не трогает:
// установка требует прав администратора, файлы запускает пользователь.

// systemdUnitTemplate - юнит systemd: %[1]s - описание, %[2]s - путь к
// программе, %[3]s - её каталог.
const systemdUnitTemplate = `[Unit]
Description=%[1]s
After=network.target

[Service]
ExecStart=%[2]s
WorkingDirectory=%[3]s
Restart=on-failure

[Install]
WantedBy=multi-user.target
`

// windowsInstallTemplate - регистрация и запуск службы через sc.exe: %[1]s -
// имя службы, %[2]s - путь к программе, %[3]s - отображаемое имя.
// "chcp 65001" - файл в UTF-8, отображаемое имя может быть кириллицей.
const windowsInstallTemplate = "@echo off\r\n" +
	"chcp 65001 >nul\r\n" +
	"sc.exe create \"%[1]s\" binPath= \"%[2]s\" start= auto DisplayName= \"%[3]s\"\r\n" +
	"sc.exe start \"%[1]s\"\r\n"

// windowsUninstallTemplate - остановка и удаление службы: %[1]s - имя.
const windowsUninstallTemplate = "@echo off\r\n" +
	"sc.exe stop \"%[1]s\"\r\n" +
	"sc.exe delete \"%[1]s\"\r\n"

// writeServiceFiles пишет файлы установки службы для текущей ОС рядом с
// binPath и возвращает их пути. projName - имя проекта (отображаемое имя
// как есть), serviceName - "сборка.имя_службы", "" - транслитерация
// projName латиницей (cbackend.ASCIIName).
func writeServiceFiles(binPath, projName, serviceName string) ([]string, error) {
	absBin, err := filepath.Abs(binPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось определить путь к программе службы: %w", err)
	}
	dir := filepath.Dir(absBin)
	name := serviceName
	if name == "" {
		name = cbackend.ASCIIName(projName)
	}
	files := map[string]string{}
	switch runtime.GOOS {
	case "windows":
		files[name+"_установить.cmd"] = fmt.Sprintf(windowsInstallTemplate, name, absBin, projName)
		files[name+"_удалить.cmd"] = fmt.Sprintf(windowsUninstallTemplate, name)
	case "linux":
		files[name+".service"] = fmt.Sprintf(systemdUnitTemplate, projName, absBin, dir)
	default:
		return nil, nil
	}
	var paths []string
	for file, content := range files {
		path := filepath.Join(dir, file)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("не удалось записать %s: %w", path, err)
		}
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths, nil
}
