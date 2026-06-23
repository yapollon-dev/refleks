Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define REFLEKS_OPTIONAL_REG_KEY "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!define REFLEKS_FFMPEG_SELECTED_VALUE "BundledFFmpegSelected"
!define REFLEKS_OBS_INSTALLER_URL "https://github.com/obsproject/obs-studio/releases/download/32.1.2/OBS-Studio-32.1.2-Windows-x64-Installer.exe"
!define REFLEKS_OBS_INSTALLER_SHA256 "94d180c1fc481ccc307b95513f795d088d63ac4f61ad3253c2ac0d94d0844110"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"

Var ObsDetected

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_COMPONENTS # Optional recording dependencies.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\\..\\bin\\refleks-${INFO_PRODUCTVERSION}-windows-${ARCH}-installer.exe" # Name of the installer's file.
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
   SetRegView 64
   Call RestoreOptionalComponentChoices
   Call DetectOBSForComponents
FunctionEnd

Section "!${INFO_PRODUCTNAME}" SecCore
    SectionIn RO
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "FFmpeg for replay trimming" SecFFmpeg
    SetOutPath "$INSTDIR\ffmpeg"
    File /r "resources\ffmpeg\*.*"
    SetOutPath "$INSTDIR\THIRD_PARTY_NOTICES\ffmpeg"
    File /r "resources\ffmpeg-notices\*.*"
SectionEnd

Section /o "OBS Studio setup" SecOBS
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File "/oname=$PLUGINSDIR\refleks_obs_optional_setup.ps1" "resources\obs_optional_setup.ps1"
    DetailPrint "Checking optional OBS Studio setup..."
    nsExec::ExecToLog 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\refleks_obs_optional_setup.ps1" -DownloadUrl "${REFLEKS_OBS_INSTALLER_URL}" -ExpectedSha256 "${REFLEKS_OBS_INSTALLER_SHA256}"'
    Pop $0
    DetailPrint "OBS Studio optional setup finished with non-fatal result code $0. RefleK's installation will continue."
SectionEnd

Function RestoreOptionalComponentChoices
    ReadRegDWORD $0 HKLM "${REFLEKS_OPTIONAL_REG_KEY}" "${REFLEKS_FFMPEG_SELECTED_VALUE}"
    IfErrors done
    SectionGetFlags ${SecFFmpeg} $1
    IntCmp $0 0 deselect select select
    deselect:
        IntOp $1 $1 & 0xFFFFFFFE
        SectionSetFlags ${SecFFmpeg} $1
        Goto done
    select:
        IntOp $1 $1 | ${SF_SELECTED}
        SectionSetFlags ${SecFFmpeg} $1
    done:
FunctionEnd

Function DetectOBSForComponents
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File "/oname=$PLUGINSDIR\refleks_obs_optional_setup.ps1" "resources\obs_optional_setup.ps1"
    nsExec::ExecToStack 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\refleks_obs_optional_setup.ps1" -DetectOnly'
    Pop $0
    Pop $1
    StrCpy $ObsDetected "0"
    StrCmp $0 "0" detected missing
    detected:
        StrCpy $ObsDetected "1"
        SectionSetText ${SecOBS} "OBS Studio setup (already detected)"
        Goto done
    missing:
        SectionSetText ${SecOBS} "OBS Studio setup (not detected)"
    done:
FunctionEnd

Function .onInstSuccess
    SetRegView 64
    SectionGetFlags ${SecFFmpeg} $0
    IntOp $0 $0 & ${SF_SELECTED}
    IntCmp $0 0 ffmpegOff ffmpegOn ffmpegOn
    ffmpegOff:
        WriteRegDWORD HKLM "${REFLEKS_OPTIONAL_REG_KEY}" "${REFLEKS_FFMPEG_SELECTED_VALUE}" 0
        RMDir /r "$INSTDIR\ffmpeg"
        Goto done
    ffmpegOn:
        WriteRegDWORD HKLM "${REFLEKS_OPTIONAL_REG_KEY}" "${REFLEKS_FFMPEG_SELECTED_VALUE}" 1
    done:
FunctionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} "Install RefleK's. Required."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecFFmpeg} "Install app-owned FFmpeg for replay trimming. No PATH changes are made."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecOBS} "Optionally download, verify, and launch the official OBS Studio installer. Failure or cancellation will not roll back RefleK's."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
