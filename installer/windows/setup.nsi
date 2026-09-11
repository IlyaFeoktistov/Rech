; Установщик Rech для Windows - без admin (RequestExecutionLevel user):
; кладёт rech.exe в %LOCALAPPDATA%\Rech\bin и добавляет этот каталог в PATH
; ПОЛЬЗОВАТЕЛЯ (HKCU\Environment, не системный - системный требует admin).
; Не устанавливает C-компилятор/внешние библиотеки сам (Rech транспилирует
; в C и требует их для финальной сборки, см. README.md, "Зависимости для
; сборки") - только проверяет наличие и предупреждает, если чего-то не
; нашлось, установка не прерывается ни при каких предупреждениях.
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
;
; Проверка внешних библиотек (Boehm GC/OpenSSL/nghttp2/libpng/libjpeg/
; клиентов БД) - тем же принципом probe-компиляции/линковки, что и
; installer/unix/install.sh: крошечный probe.c компилируется и линкуется
; уже найденным компилятором ($CCTOOL) с нужным заголовком (если он у
; библиотеки настоящий, как у OpenSSL/nghttp2/PNG/JPEG - см. package doc
; internal/backend/cbackend) и "-l<имя>" - неудача компиляции/линковки
; значит библиотека отсутствует. В ОТЛИЧИЕ от Unix-версии, НЕ проверено
; вручную на настоящем Windows (нет тестовой машины под рукой) - написано
; по документированному
; поведению NSIS/nsExec, тем же уровнем доверия, что и остальные
; Windows-специфичные ветки этого компилятора, где нет возможности
; проверить их здесь и сейчас.

!include "LogicLib.nsh"

; Name/OutFile/Unicode/SetCompressor - header-уровневые директивы NSIS,
; обязаны идти ДО любых Var/Function/Section (иначе makensis падает
; "Can't change target architecture after data already got compressed or
; header already changed!" - живой найденный баг при первой сборке этого
; файла: раньше Var/Function-блоки ниже (StrContains) шли ДО этого блока,
; и это НЕ ломалось - именно НОВЫЕ Function-блоки в этой правке (ProbeLib/
; ProbeLibTwoHeaders) - и только вместе с уже присутствовавшим Var - сами
; по себе заставляют makensis зафиксировать stub раньше, чем "Unicode
; true"/"SetCompressor" успевают сработать).
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

Var CCTOOL       ; имя сработавшего компилятора (cc/gcc/clang) или "" - не найден
Var PROBE_HEADERS ; список заголовков через пробел для очередной пробы, "" - без заголовка
Var PROBE_LIBFLAGS ; флаги линковщика очередной пробы, например "-lgc"
Var PROBE_RESULT ; "1" - найдена, "0" - не найдена, заполняется ProbeLib
Var MISSING_HINTS ; накопленный текст предупреждений о ненайденных библиотеках

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

; ProbeLib - пробует скомпилировать и слинковать "$PLUGINSDIR\rech_probe.c"
; (пуст, кроме нужных #include из $PROBE_HEADERS и пустой "main") через
; $CCTOOL с флагами $PROBE_LIBFLAGS - неудача (ненулевой код возврата
; компилятора) значит недостающий заголовок ИЛИ саму библиотеку линковщик
; не нашёл. Результат - $PROBE_RESULT ("1"/"0"). $PLUGINSDIR - временный
; каталог самого NSIS-инсталлятора, очищается им же автоматически после
; завершения установки - отдельной очистки за собой (в отличие от
; install.sh, где это ручной "trap ... EXIT") не требуется, но probe.c/
; probe.exe всё равно удаляются явно, чтобы не путаться при повторных
; пробах внутри одной установки.
Function ProbeLib
    FileOpen $0 "$PLUGINSDIR\rech_probe.c" w
    ${If} $PROBE_HEADERS != ""
        FileWrite $0 "#include <$PROBE_HEADERS>$\r$\n"
    ${EndIf}
    FileWrite $0 "int main(void) { return 0; }$\r$\n"
    FileClose $0

    nsExec::ExecToStack '"$CCTOOL" "$PLUGINSDIR\rech_probe.c" $PROBE_LIBFLAGS -o "$PLUGINSDIR\rech_probe.exe"'
    Pop $0

    Delete "$PLUGINSDIR\rech_probe.c"
    Delete "$PLUGINSDIR\rech_probe.exe"

    ${If} $0 == "0"
        StrCpy $PROBE_RESULT "1"
    ${Else}
        StrCpy $PROBE_RESULT "0"
    ${EndIf}
FunctionEnd

; ProbeLibTwoHeaders - "#include" не поддерживает несколько заголовков через
; пробел в одной строке (в отличие от install.sh, где "for h in $headers"
; пишет одну строку "#include <...>" на каждый) - для jpeglib.h (нужен
; "stdio.h" ДО себя, см. package doc internal/backend/cbackend/jpeg.go, тот
; же порядок, что cbackend эмитирует в сгенерированном C) FileWrite вызван
; дважды напрямую в самом месте пробы вместо ещё одного параметра ProbeLib -
; единственный такой случай, заводить отдельный список ради одной пробы не
; стоит.
Function ProbeLibTwoHeaders
    FileOpen $0 "$PLUGINSDIR\rech_probe.c" w
    FileWrite $0 "#include <stdio.h>$\r$\n"
    FileWrite $0 "#include <$PROBE_HEADERS>$\r$\n"
    FileWrite $0 "int main(void) { return 0; }$\r$\n"
    FileClose $0

    nsExec::ExecToStack '"$CCTOOL" "$PLUGINSDIR\rech_probe.c" $PROBE_LIBFLAGS -o "$PLUGINSDIR\rech_probe.exe"'
    Pop $0

    Delete "$PLUGINSDIR\rech_probe.c"
    Delete "$PLUGINSDIR\rech_probe.exe"

    ${If} $0 == "0"
        StrCpy $PROBE_RESULT "1"
    ${Else}
        StrCpy $PROBE_RESULT "0"
    ${EndIf}
FunctionEnd

Section "Install"
    SetOutPath "$INSTDIR"
    File "rech.exe"
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; Часовые пояса (IANA tzdata, формат TZif) - собственный читатель
    ; cbackend (internal/backend/cbackend/tzdata.go, "время_как_дата_в_
    ; поясе"/"дата_в_поясе_как_время"/"д.в_поясе(...)") читает ВШИТУЮ
    ; копию (нет системного "/usr/share/zoneinfo" на Windows вообще) -
    ; путь рядом с rech.exe ($INSTDIR\zoneinfo), тот же принцип, что
    ; tzdataDir (tzdata.go) вычисляет независимо во время "rech build"/
    ; "rech run" на этой машине.
    SetOutPath "$INSTDIR\zoneinfo"
    File /r "..\..\runtime\zoneinfo\*.*"
    SetOutPath "$INSTDIR"

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

    ; Компилятор - тот же порядок поиска, что и internal/toolchain.CC (env
    ; $CC не проверяется тут отдельно - установщик работает от имени
    ; пользователя ДО того, как тот запустит rech, переменная $CC - его
    ; личный выбор на будущее, не влияет на probe здесь): cc, затем gcc,
    ; затем clang. $CCTOOL несёт имя сработавшего - используется ДАЛЬШЕ
    ; для проб всех остальных библиотек, "" - ни один не найден, все пробы
    ; библиотек ниже тогда пропускаются целиком (без компилятора проверить
    ; линковку нечем, тот же принцип, что "if [ -n "$CC_BIN" ]" в
    ; install.sh).
    StrCpy $CCTOOL "cc"
    nsExec::ExecToStack 'where $CCTOOL'
    Pop $2
    ${If} $2 != "0"
        StrCpy $CCTOOL "gcc"
        nsExec::ExecToStack 'where $CCTOOL'
        Pop $2
        ${If} $2 != "0"
            StrCpy $CCTOOL "clang"
            nsExec::ExecToStack 'where $CCTOOL'
            Pop $2
        ${EndIf}
    ${EndIf}
    ${If} $2 != "0"
        StrCpy $CCTOOL ""
        MessageBox MB_OK|MB_ICONEXCLAMATION "Не найден C-компилятор (cc/gcc/clang) в PATH.$\r$\n$\r$\nRech транспилирует программы в C и требует системный компилятор для финальной сборки (rech build/rech run). Установите MinGW-w64, например:$\r$\n  winget install BrechtSanders.WinLibs.POSIX.UCRT$\r$\nи добавьте его bin-каталог в PATH."
    ${EndIf}

    ${If} $CCTOOL != ""
        StrCpy $MISSING_HINTS ""

        ; Boehm GC (libgc) - нужна КАЖДОЙ Rech-программе хотя бы с одной
        ; ссылочной "сущность"/"класс" - см. README.md, "Зависимости для
        ; сборки" (там же - способ собрать bdwgc из исходников под
        ; MinGW-w64, если vcpkg не используется).
        StrCpy $PROBE_HEADERS "gc.h"
        StrCpy $PROBE_LIBFLAGS "-lgc"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - Boehm GC (libgc, нужна КАЖДОЙ программе с сущностью/классом): vcpkg install bdwgc, либо собрать bdwgc из исходников под MinGW-w64 (см. README.md)"
        ${EndIf}

        ; Клиентские библиотеки БД - тем же принципом, что install.sh
        ; (cbackend не включает их настоящие заголовки - internal/backend/
        ; cbackend/db.go, - проба без #include, только линковка по имени).
        StrCpy $PROBE_HEADERS ""
        StrCpy $PROBE_LIBFLAGS "-lsqlite3"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - SQLite ($\"БД$\" со схемой sqlite:): vcpkg install sqlite3"
        ${EndIf}

        StrCpy $PROBE_LIBFLAGS "-lpq"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - PostgreSQL ($\"БД$\" со схемой postgres://): vcpkg install libpq (или libpq.lib из поставки PostgreSQL)"
        ${EndIf}

        StrCpy $PROBE_LIBFLAGS "-lmysqlclient"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - MySQL ($\"БД$\" со схемой mysql://): vcpkg install libmysql (или MySQL Connector/C)"
        ${EndIf}

        ; OpenSSL/nghttp2/libpng/libjpeg - cbackend ДЕЙСТВИТЕЛЬНО включает
        ; настоящие заголовки этих библиотек, проба честно #include их же
        ; (тот же принцип, что новый блок install.sh).
        StrCpy $PROBE_HEADERS "openssl/ssl.h"
        StrCpy $PROBE_LIBFLAGS "-lssl -lcrypto"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - OpenSSL (TLS/$\"слушать_tcp_защищённо$\"/$\"зашифровать$\"/$\"расшифровать$\"): vcpkg install openssl"
        ${EndIf}

        StrCpy $PROBE_HEADERS "nghttp2/nghttp2.h"
        StrCpy $PROBE_LIBFLAGS "-lnghttp2"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - nghttp2 (HTTP/2- и gRPC-функции, $\"http2_*$\"/$\"grpc_*$\"/$\"слушать_grpcs$\"): vcpkg install nghttp2 (или MSYS2: pacman -S mingw-w64-x86_64-libnghttp2)"
        ${EndIf}

        StrCpy $PROBE_HEADERS "png.h"
        StrCpy $PROBE_LIBFLAGS "-lpng"
        Call ProbeLib
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - libpng ($\"декодировать_изображение$\"/$\"закодировать_изображение$\" с ФорматИзображения.PNG): vcpkg install libpng"
        ${EndIf}

        ; jpeglib.h требует "stdio.h" ДО себя (FILE) - отдельная функция
        ; (ProbeLibTwoHeaders), см. её package doc выше.
        StrCpy $PROBE_HEADERS "jpeglib.h"
        StrCpy $PROBE_LIBFLAGS "-ljpeg"
        Call ProbeLibTwoHeaders
        ${If} $PROBE_RESULT == "0"
            StrCpy $MISSING_HINTS "$MISSING_HINTS$\r$\n  - libjpeg ($\"декодировать_изображение$\"/$\"закодировать_изображение$\" с ФорматИзображения.JPEG): vcpkg install libjpeg-turbo (или MSYS2: pacman -S mingw-w64-x86_64-libjpeg-turbo)"
        ${EndIf}

        ${If} $MISSING_HINTS != ""
            MessageBox MB_OK|MB_ICONINFORMATION "Rech установлен, но не все ОПЦИОНАЛЬНЫЕ библиотеки для сборки Rech-программ найдены (нужны только программе, реально использующей соответствующую фичу):$\r$\n$MISSING_HINTS"
        ${EndIf}
    ${EndIf}
SectionEnd

Section "Uninstall"
    Delete "$INSTDIR\rech.exe"
    Delete "$INSTDIR\uninstall.exe"
    RMDir /r "$INSTDIR\zoneinfo"
    RMDir "$INSTDIR"
SectionEnd
