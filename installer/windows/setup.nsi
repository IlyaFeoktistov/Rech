; Установщик Rech для Windows - без admin (RequestExecutionLevel user):
; кладёт rech.exe в %LOCALAPPDATA%\Rech\bin и добавляет этот каталог в PATH
; ПОЛЬЗОВАТЕЛЯ (HKCU\Environment, не системный - системный требует admin).
; Не устанавливает C-компилятор сам (Rech транспилирует в C и требует его
; для финальной сборки, см. docs/MANUAL.md, "Зависимости для сборки") -
; только проверяет наличие cc/gcc/clang в PATH и предупреждает, если не
; нашёл, установка не прерывается.
;
; Собрать: положить rech.exe (go build -o rech.exe ./cmd/rech, GOOS=windows)
; рядом с этим файлом, затем "makensis setup.nsi" - см. build.ps1.
;
; Известное ограничение: повторная установка (или установка через
; несколько версий) может добавить в PATH дублирующуюся запись каталога -
; безвредно (PATH ищет по порядку, дубликат просто не используется), но не
; идеально чисто - устранение требует парсинга PATH построково (WordFunc.nsh),
; сознательно не сделано ради простоты этого срезa.

!include "LogicLib.nsh"

Name "Rech"
OutFile "..\..\rech-setup.exe"
InstallDir "$LOCALAPPDATA\Rech\bin"
RequestExecutionLevel user
Unicode true
SetCompressor /SOLID lzma

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

!define WM_WININICHANGE 0x001A
!define HWND_BROADCAST 0xFFFF

Section "Install"
    SetOutPath "$INSTDIR"
    File "rech.exe"
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ReadRegStr $0 HKCU "Environment" "Path"
    ${If} $0 == ""
        WriteRegExpandStr HKCU "Environment" "Path" "$INSTDIR"
    ${Else}
        WriteRegExpandStr HKCU "Environment" "Path" "$0;$INSTDIR"
    ${EndIf}
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000

    nsExec::ExecToStack 'where cc'
    Pop $2
    ${If} $2 != "0"
        nsExec::ExecToStack 'where gcc'
        Pop $2
        ${If} $2 != "0"
            nsExec::ExecToStack 'where clang'
            Pop $2
        ${EndIf}
    ${EndIf}
    ${If} $2 != "0"
        MessageBox MB_OK|MB_ICONEXCLAMATION "Не найден C-компилятор (cc/gcc/clang) в PATH.$\r$\n$\r$\nRech транспилирует программы в C и требует системный компилятор для финальной сборки (rech build/rech run). Установите MinGW-w64, например:$\r$\n  winget install BrechtSanders.WinLibs.POSIX.UCRT$\r$\nи добавьте его bin-каталог в PATH."
    ${EndIf}
SectionEnd

Section "Uninstall"
    Delete "$INSTDIR\rech.exe"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR"
SectionEnd
