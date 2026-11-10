Unicode True
!include "MUI2.nsh"
!include "x64.nsh"
!ifndef PRODUCT_VERSION
!error "PRODUCT_VERSION is required"
!endif
!ifndef PACKAGE_DIR
!error "PACKAGE_DIR is required"
!endif
!ifndef OUTPUT_FILE
!error "OUTPUT_FILE is required"
!endif
Name "Tunnelix"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\Tunnelix"
InstallDirRegKey HKCU "Software\Tunnelix" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
!ifdef ICON_FILE
Icon "${ICON_FILE}"
UninstallIcon "${ICON_FILE}"
!endif
VIProductVersion "${PRODUCT_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "Tunnelix"
VIAddVersionKey /LANG=1033 "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "FileDescription" "Tunnelix Setup"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Copyright (c) 2026 Alan"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\tunnelix.exe"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${PACKAGE_DIR}\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
LangString QuitBeforeUninstall ${LANG_ENGLISH} "Quit Tunnelix, then retry uninstalling."
LangString QuitBeforeUninstall ${LANG_SIMPCHINESE} "请退出 Tunnelix，再重试卸载。"
!ifdef SIGN_SCRIPT
!uninstfinalize 'powershell.exe -NoProfile -NonInteractive -File "${SIGN_SCRIPT}" "%1"' = 0
!endif
Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Tunnelix requires 64-bit Windows."
    Abort
  ${EndIf}
  SetRegView 64
  SetShellVarContext current
  !insertmacro MUI_LANGDLL_DISPLAY
FunctionEnd
Function un.onInit
  SetRegView 64
  SetShellVarContext current
FunctionEnd
Section "Tunnelix"
  SetOutPath "$INSTDIR"
  SetOverwrite on
  File "${PACKAGE_DIR}\tunnelix.exe"
  File "${PACKAGE_DIR}\LICENSE"
  File "${PACKAGE_DIR}\THIRD_PARTY_LICENSES.txt"
  File "${PACKAGE_DIR}\THIRD_PARTY_NOTICES.md"
  File "${PACKAGE_DIR}\README.md"
  File "${PACKAGE_DIR}\README.zh-CN.md"
  File "${PACKAGE_DIR}\tunnelix.png"
  File "${PACKAGE_DIR}\tunnelix.ico"
  SetOutPath "$INSTDIR\docs"
  File "${PACKAGE_DIR}\docs\install.md"
  File "${PACKAGE_DIR}\docs\development.md"
  File "${PACKAGE_DIR}\docs\releasing.md"
  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\Tunnelix"
  CreateShortcut "$SMPROGRAMS\Tunnelix\Tunnelix.lnk" "$INSTDIR\tunnelix.exe" "" "$INSTDIR\tunnelix.ico" 0
  CreateShortcut "$SMPROGRAMS\Tunnelix\Uninstall.lnk" "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\Tunnelix" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "DisplayName" "Tunnelix"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "Publisher" "Alan"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "DisplayIcon" "$INSTDIR\tunnelix.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix" "NoRepair" 1
SectionEnd
Section "Uninstall"
  ; Leave %APPDATA%\Tunnelix, OS credentials, and any user-added files untouched.
  IfFileExists "$INSTDIR\tunnelix.exe" 0 un.files
  un.retry:
    ClearErrors
    Delete "$INSTDIR\tunnelix.exe"
    IfErrors 0 un.files
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(QuitBeforeUninstall)" IDRETRY un.retry
    Abort
  un.files:
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\THIRD_PARTY_LICENSES.txt"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\README.zh-CN.md"
  Delete "$INSTDIR\tunnelix.png"
  Delete "$INSTDIR\tunnelix.ico"
  Delete "$INSTDIR\docs\install.md"
  Delete "$INSTDIR\docs\development.md"
  Delete "$INSTDIR\docs\releasing.md"
  ; Remove documentation from older installations as well.
  Delete "$INSTDIR\docs\updates.md"
  Delete "$INSTDIR\docs\validation.md"
  Delete "$INSTDIR\docs\implementation-scope.md"
  RMDir "$INSTDIR\docs"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Tunnelix\Tunnelix.lnk"
  Delete "$SMPROGRAMS\Tunnelix\Uninstall.lnk"
  RMDir "$SMPROGRAMS\Tunnelix"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Tunnelix"
  DeleteRegKey HKCU "Software\Tunnelix"
SectionEnd
