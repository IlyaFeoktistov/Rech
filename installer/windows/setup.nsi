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
; Повторная установка (в т. ч. поверх другой версии) не добавляет
; дублирующуюся запись каталога в PATH - StrContains ищет "$INSTDIR" как
; отдельный, окружённый ";" сегмент уже записанного PATH (не просто
; подстроку - иначе более длинный каталог, содержащий $INSTDIR как часть
; своего имени, ложно засчитался бы совпадением) перед тем, как что-либо
; дописывать.

!include "LogicLib.nsh"

; StrContains - $R0 (needle) в $R1 (haystack) - "1" в $R0 при совпадении,
; "0" - иначе. Классический самодостаточный NSIS-снипет (посимвольный
; сдвиг окна длины needle по haystack) - без внешних *.nsh, которых может
; не быть у makensis по умолчанию.
Function StrContains
    Exch $R0 ; needle
    Exch
    Exch $R1 ; haystack
    Push $R2
    Push $R3
    Push $R4
    Push $R5

    StrLen $R2 $R0
    StrCpy $R3 0

    loop:
        StrCpy $R4 $R1 $R2 $R3
        StrCmp $R4 $R0 found
        StrCmp $R4 "" notfound
        IntOp $R3 $R3 + 1
        Goto loop

    found:
        StrCpy $R0 "1"
        Goto done
    notfound:
        StrCpy $R0 "0"

    done:
        Pop $R5
        Pop $R4
        Pop $R3
        Pop $R2
        Pop $R1
        Exch $R0
FunctionEnd

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
        SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
    ${Else}
        Push ";$0;"
        Push ";$INSTDIR;"
        Call StrContains
        Pop $1
        ${If} $1 == "0"
            WriteRegExpandStr HKCU "Environment" "Path" "$0;$INSTDIR"
            SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
        ${EndIf}
    ${EndIf}

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
