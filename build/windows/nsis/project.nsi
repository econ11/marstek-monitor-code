Unicode true

####
## Marstek Monitor installer (plan 3, Task 6). Per-user, no admin, no firewall rule, unsigned.
##
## Built by `wails3 task package VERSION=x.y.z[-pre]` (build/windows/Taskfile.yml), which passes:
##   -DARG_WAILS_AMD64_BINARY=<bin\marstek-monitor.exe>
##   -DAPP_VERSION=<VERSION as given>      e.g. 0.0.1-test (file name, ARP DisplayVersion)
##   -DINFO_PRODUCTVERSION=<x.y.z>         numeric part, for the exe's version resource
## By hand: makensis -INPUTCHARSET UTF8 -DAPP_VERSION=0.0.1-test -DINFO_PRODUCTVERSION=0.0.1
##            -DARG_WAILS_AMD64_BINARY=..\..\..\bin\marstek-monitor.exe project.nsi
##
## Command line:
##   /S          silent
##   /relaunch   the one-click update (app.InstallerArgs = /S /relaunch): wait for the running app
##               to quit (single-instance mutex free), install, start the new version; if the
##               files cannot be replaced, start the previous one again: the installed exe,
##               else the exe that started the update (env MARSTEK_PREV_EXE, app.PrevExeEnv;
##               only run, never stopped, moved or deleted)
##   /D=<dir>    install folder (must be last; NSIS standard)
##
## The installer never touches %LOCALAPPDATA%\MarstekMonitor (the user's data) and never writes
## the autostart Run value: the app manages it (and repoints it to the installed exe, Task 6).
## The uninstaller deletes the Run value only when it points into $INSTDIR.
####

!define WAILS_INSTALL_SCOPE "user"
!define REQUEST_EXECUTION_LEVEL "user"
# The ARP entry; app/autostart_windows.go (uninstallKeyPath) reads InstallLocation from it.
!define UNINST_KEY_NAME "MarstekMonitor"
# The real app's single-instance mutex (app.FirstInstance("MarstekMonitor"); -simulate uses another).
!define APP_MUTEX "Local\MarstekMonitor"
# The app's autostart value (app/autostart_windows.go).
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define RUN_VALUE "MarstekMonitor"

!ifndef APP_VERSION
    !define APP_VERSION "0.0.1"
!endif

!include "wails_tools.nsh"
!include "LogicLib.nsh"

# The version resource needs four numbers.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${APP_VERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "$(RunApp)"
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

# The first language is the fallback: English unless the Windows UI is Ukrainian (.onInit).
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Ukrainian"

LangString RunApp ${LANG_ENGLISH} "Run Marstek Monitor"
LangString RunApp ${LANG_UKRAINIAN} "Запустити Marstek Monitor"
LangString CloseApp ${LANG_ENGLISH} "Marstek Monitor is running. Quit it (right-click the tray icon → Quit), then click Retry."
LangString CloseApp ${LANG_UKRAINIAN} "Marstek Monitor запущено. Закрийте його (клацніть іконку в треї правою кнопкою → «Вийти») і натисніть «Повторити»."
LangString InstallFailed ${LANG_ENGLISH} "Could not replace the program files. The previous version is left as it was."
LangString InstallFailed ${LANG_UKRAINIAN} "Не вдалося замінити файли програми. Попередню версію залишено як була."
LangString DataKept ${LANG_ENGLISH} "Your data is kept in %LOCALAPPDATA%\MarstekMonitor"
LangString DataKept ${LANG_UKRAINIAN} "Ваші дані збережено в %LOCALAPPDATA%\MarstekMonitor"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${APP_VERSION}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
# An update goes where the previous install went (also a custom /D= one).
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
ShowInstDetails show
ShowUninstDetails show

Var Relaunch ; "1" with /relaunch
Var HadOld   ; "1" when an installed exe was there before
Var PrevExe  ; with /relaunch: the exe that started the update (MARSTEK_PREV_EXE), may be ""

# uk when the Windows UI language is Ukrainian (primary language 0x22), else en.
!macro SELECT_LANGUAGE
    System::Call 'kernel32::GetUserDefaultUILanguage() i .r0'
    IntOp $0 $0 & 0x3FF
    ${If} $0 = 34
        StrCpy $LANGUAGE ${LANG_UKRAINIAN}
    ${Else}
        StrCpy $LANGUAGE ${LANG_ENGLISH}
    ${EndIf}
!macroend

# Functions shared by the installer and the uninstaller (UN is "" or "un.").
!macro APP_PROCESS_FUNCS UN
    # Runs PowerShell on the processes started from $INSTDIR\<exe> only (the path goes in through
    # an environment variable, so no quoting). $1 = the pipeline tail; $0 = exit code.
    Function ${UN}AppInDirPS
        System::Call 'kernel32::SetEnvironmentVariable(t "MARSTEK_SETUP_EXE", t "$INSTDIR\${PRODUCT_EXECUTABLE}")'
        ${If} ${RunningX64}
            ${DisableX64FSRedirection} ; 64-bit PowerShell sees the 64-bit app's Path
        ${EndIf}
        nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -Command "$$p = $$env:MARSTEK_SETUP_EXE; $$a = @(Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($$p)) -ErrorAction SilentlyContinue | Where-Object { $$_.Path -eq $$p }); $1"'
        ${If} ${RunningX64}
            ${EnableX64FSRedirection}
        ${EndIf}
        Pop $0
        Pop $1 ; output, unused
    FunctionEnd

    # $0 = 1 when the app runs from $INSTDIR.
    Function ${UN}AppRunningInDir
        StrCpy $1 'if ($$a.Count -gt 0) { exit 1 }; exit 0'
        Call ${UN}AppInDirPS
        ${If} $0 == 1
            StrCpy $0 1
        ${Else}
            StrCpy $0 0
        ${EndIf}
    FunctionEnd

    # Kills the app started from $INSTDIR only (never a copy elsewhere), then waits up to 5 s.
    Function ${UN}KillAppInDir
        DetailPrint "Stopping $INSTDIR\${PRODUCT_EXECUTABLE}"
        StrCpy $1 '$$a | Stop-Process -Force; exit 0'
        Call ${UN}AppInDirPS
        StrCpy $2 0
        ${Do}
            Call ${UN}AppRunningInDir
            ${If} $0 != 1
            ${OrIf} $2 >= 20
                ${Break}
            ${EndIf}
            Sleep 250
            IntOp $2 $2 + 1
        ${Loop}
    FunctionEnd

    # Before files are replaced or removed. Interactive: ask the user to quit the running copy
    # (installing: any real copy, since the installed one cannot start beside it; uninstalling:
    # the one in $INSTDIR). Silent: kill only the copy in $INSTDIR.
    Function ${UN}CloseRunningApp
        ${Do}
            Call ${UN}AppRunningInDir
            StrCpy $3 $0
            !if "${UN}" == ""
                Call AppMutexHeld
            !else
                StrCpy $0 0
            !endif
            ${If} $3 != 1
            ${AndIf} $0 != 1
                ${Break}
            ${EndIf}
            ${If} ${Silent}
                ${If} $3 == 1
                    Call ${UN}KillAppInDir
                ${EndIf}
                ${Break} ; a copy outside $INSTDIR is not ours to stop
            ${EndIf}
            MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(CloseApp)" IDRETRY retry
            Quit
            retry:
        ${Loop}
    FunctionEnd
!macroend
!insertmacro APP_PROCESS_FUNCS ""
!insertmacro APP_PROCESS_FUNCS "un."

# $0 = 1 while some real copy of the app runs (anywhere in this session: the mutex is Local\).
Function AppMutexHeld
    StrCpy $0 0
    System::Call 'kernel32::OpenMutexW(i 0x00100000, i 0, w "${APP_MUTEX}") p .r1'
    ${If} $1 P<> 0
        System::Call 'kernel32::CloseHandle(p r1)'
        StrCpy $0 1
    ${EndIf}
FunctionEnd

# /relaunch: the app starts us and then quits, which takes a few seconds. Wait (up to 30 s) for
# the single-instance mutex to be free, so the relaunched copy is not turned away by the old one.
Function WaitForAppExit
    StrCpy $2 0
    ${Do}
        Call AppMutexHeld
        ${If} $0 != 1
        ${OrIf} $2 >= 120
            ${Break}
        ${EndIf}
        Sleep 250
        IntOp $2 $2 + 1
    ${Loop}
FunctionEnd

Function .onInit
    !insertmacro wails.checkArchitecture
    !insertmacro SELECT_LANGUAGE
    StrCpy $Relaunch 0
    ${GetParameters} $R0
    ClearErrors
    ${GetOptions} $R0 "/relaunch" $R1
    ${IfNot} ${Errors}
        StrCpy $Relaunch 1
    ${EndIf}
    ReadEnvStr $PrevExe MARSTEK_PREV_EXE
FunctionEnd

Function un.onInit
    !insertmacro SELECT_LANGUAGE
FunctionEnd

Section
    SetShellVarContext current

    ${If} $Relaunch == 1
        Call WaitForAppExit
    ${EndIf}
    Call CloseRunningApp
    ${If} $Relaunch == 1
        Call WaitForAppExit ; after a kill the mutex goes with the process
    ${EndIf}

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    # Replace the exe so that a failure leaves the previous one in place: move it aside first
    # (a quitting process can hold it a moment: retry for 5 s), copy, then drop the old one.
    StrCpy $HadOld 0
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        StrCpy $HadOld 1
        Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old"
        StrCpy $2 0
        ${Do}
            ClearErrors
            Rename "$INSTDIR\${PRODUCT_EXECUTABLE}" "$INSTDIR\${PRODUCT_EXECUTABLE}.old"
            ${IfNot} ${Errors}
                ${Break}
            ${EndIf}
            IntOp $2 $2 + 1
            ${If} $2 >= 10
                Goto failed
            ${EndIf}
            Sleep 500
        ${Loop}
    ${EndIf}

    # `try`: a file that cannot be written is skipped with the error flag set, never the
    # Abort/Retry/Ignore box (its Abort would skip the rollback below and leave only the .old).
    ClearErrors
    SetOverwrite try
    File "/oname=${PRODUCT_EXECUTABLE}" "${ARG_WAILS_AMD64_BINARY}"
    SetOverwrite on
    ${If} ${Errors}
    ${OrIfNot} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
        ${If} $HadOld == 1
            Rename "$INSTDIR\${PRODUCT_EXECUTABLE}.old" "$INSTDIR\${PRODUCT_EXECUTABLE}"
        ${EndIf}
        Goto failed
    ${EndIf}
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old" ; may stay if still in use: removed next time

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    # «Програми й компоненти» (per user).
    WriteUninstaller "$INSTDIR\uninstall.exe"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${APP_VERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"

    ${If} $Relaunch == 1
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
    ${EndIf}
    Return

failed:
    DetailPrint "$(InstallFailed)"
    # The update must not leave the user without the app: start the previous version again,
    # the installed one if there was one, else the exe that started the update (e.g. a
    # hand-copied exe on its first update).
    ${If} $Relaunch == 1
        ${If} $HadOld == 1
        ${AndIf} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
            Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
        ${ElseIf} $PrevExe != ""
        ${AndIf} ${FileExists} "$PrevExe"
            Exec '"$PrevExe"'
        ${EndIf}
    ${EndIf}
    MessageBox MB_OK|MB_ICONSTOP "$(InstallFailed)" /SD IDOK
    SetErrorLevel 2
    Abort
SectionEnd

Section "uninstall"
    SetShellVarContext current

    Call un.CloseRunningApp

    # Only our files: $INSTDIR may be a folder the user picked, so no recursive delete.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    DeleteRegKey HKCU "${UNINST_KEY}"

    # Autostart: only when it starts the exe in $INSTDIR (a hand-copied exe's value stays).
    ReadRegStr $0 HKCU "${RUN_KEY}" "${RUN_VALUE}"
    ${If} $0 != ""
        StrCpy $1 '"$INSTDIR\'
        StrLen $2 $1
        StrCpy $3 $0 $2
        StrCpy $4 '$INSTDIR\'
        StrLen $5 $4
        StrCpy $6 $0 $5
        ${If} $3 == $1 ; == is case-insensitive
        ${OrIf} $6 == $4
            DeleteRegValue HKCU "${RUN_KEY}" "${RUN_VALUE}"
        ${EndIf}
    ${EndIf}

    # %LOCALAPPDATA%\MarstekMonitor (database, log, backups) is never touched; neither is the
    # WebView2 cache in %APPDATA%\${PRODUCT_EXECUTABLE}, which a hand-copied exe shares.
    DetailPrint "$(DataKept)"
SectionEnd

Function un.onUninstSuccess
    MessageBox MB_OK|MB_ICONINFORMATION "$(DataKept)" /SD IDOK
FunctionEnd
