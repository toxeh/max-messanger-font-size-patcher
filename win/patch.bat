@echo off
setlocal enabledelayedexpansion

:: =============================================================================
:: Max.app Font Patcher for Windows
:: =============================================================================
:: Usage:
::   patch.bat                          -- patch with defaults (size=13)
::   patch.bat --size 14                -- custom size  
::   patch.bat --style list             -- list all available styles
::   patch.bat --path "C:\custom\path"  -- custom install path
:: =============================================================================

:: Defaults
set "STYLES=BodyStrong,MarkdownMessageMonospace"
set "SIZE=13"
set "APP_DIR="

:: Parse arguments
:parse_args
if "%~1"=="" goto :done_args
if /i "%~1"=="--path"  ( set "APP_DIR=%~2" & shift & shift & goto :parse_args )
if /i "%~1"=="-p"      ( set "APP_DIR=%~2" & shift & shift & goto :parse_args )
if /i "%~1"=="--style" ( set "STYLES=%~2"  & shift & shift & goto :parse_args )
if /i "%~1"=="-s"      ( set "STYLES=%~2"  & shift & shift & goto :parse_args )
if /i "%~1"=="--size"  ( set "SIZE=%~2"    & shift & shift & goto :parse_args )
if /i "%~1"=="-z"      ( set "SIZE=%~2"    & shift & shift & goto :parse_args )
echo Unknown parameter: %~1
exit /b 1
:done_args

:: Auto-detect Max install path if not specified
if "%APP_DIR%"=="" (
    :: Try AppData\Local first (per-user install)
    if exist "%LOCALAPPDATA%\max\max.exe" (
        set "APP_DIR=%LOCALAPPDATA%\max"
    ) else if exist "%LOCALAPPDATA%\Max\max.exe" (
        set "APP_DIR=%LOCALAPPDATA%\Max"
    ) else if exist "%ProgramFiles%\max\max.exe" (
        set "APP_DIR=%ProgramFiles%\max"
    ) else if exist "%ProgramFiles%\Max\max.exe" (
        set "APP_DIR=%ProgramFiles%\Max"
    ) else (
        echo ERROR: Could not auto-detect Max installation.
        echo Please specify the path: patch.bat --path "C:\path\to\max"
        exit /b 1
    )
)

set "TARGET=%APP_DIR%\max.exe"
set "BAK=%APP_DIR%\max.exe.bak"

:: List mode
if /i "%STYLES%"=="list" (
    echo Reading available font styles from %TARGET%...
    font-patcher.exe -binary "%TARGET%" -style list
    exit /b 0
)

echo === Max Font Patcher (Windows) ===
echo App:    %APP_DIR%
echo Styles: %STYLES%
echo Size:   %SIZE% px
echo --------------------------------

if not exist "%TARGET%" (
    echo ERROR: File %TARGET% not found. Is Max installed?
    exit /b 1
)

:: 1. Create backup of the original clean binary (only once)
if not exist "%BAK%" (
    echo Creating first backup of original file...
    copy /Y "%TARGET%" "%BAK%" >nul
    if errorlevel 1 (
        echo ERROR: Failed to create backup. Try running as Administrator.
        exit /b 1
    )
    echo Backup created: %BAK%
) else (
    echo Backup already exists: %BAK%
)

:: 2. Restore clean binary from backup before patching
:: This prevents "patched JSON is too large" on repeated runs
echo Restoring clean binary from backup...
copy /Y "%BAK%" "%TARGET%" >nul
if errorlevel 1 (
    echo ERROR: Failed to restore backup. Try running as Administrator.
    exit /b 1
)

:: 3. Apply the patch
set /a LH=%SIZE% + 4
echo Patching font sizes (px=%SIZE%, lh=%LH%) into JSON resource...
font-patcher.exe -binary "%TARGET%" -style "%STYLES%" -size %SIZE% -line-height %LH% -no-backup
if errorlevel 1 (
    echo ERROR: Patching failed.
    exit /b 1
)

echo.
echo SUCCESS! You can now launch Max.
echo.

endlocal
