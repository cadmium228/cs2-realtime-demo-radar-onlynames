@echo off
chcp 65001 >nul
echo [X] ща решим, погоди
echo.
taskkill /F /IM "radarik.exe" >nul 2>&1
taskkill /F /IM "go.exe" >nul 2>&1
echo Поиск Counter-Strike 2...

for /f "tokens=2*" %%a in ('reg query "HKLM\SOFTWARE\WOW6432Node\Valve\Steam" /v InstallPath 2^>nul') do set "steampath=%%b"

if not defined steampath (
    for /f "tokens=2*" %%a in ('reg query "HKLM\SOFTWARE\Valve\Steam" /v InstallPath 2^>nul') do set "steampath=%%b"
)

set "cspath="

if defined steampath (
    if exist "%steampath%\steamapps\common\Counter-Strike Global Offensive\game\csgo" (
        set "cspath=%steampath%\steamapps\common\Counter-Strike Global Offensive\game\csgo"
    )
)

if not defined cspath (
    echo Поиск на дисках, подождите...
    for %%d in (C D E F G) do (
        if exist "%%d:\SteamLibrary\steamapps\common\Counter-Strike Global Offensive\game\csgo" (
            set "cspath=%%d:\SteamLibrary\steamapps\common\Counter-Strike Global Offensive\game\csgo"
        )
        if exist "%%d:\Steam\steamapps\common\Counter-Strike Global Offensive\game\csgo" (
            set "cspath=%%d:\Steam\steamapps\common\Counter-Strike Global Offensive\game\csgo"
        )
        if exist "%%d:\Games\Steam\steamapps\common\Counter-Strike Global Offensive\game\csgo" (
            set "cspath=%%d:\Games\Steam\steamapps\common\Counter-Strike Global Offensive\game\csgo"
        )
    )
)

if not defined cspath (
    color 0C
    echo Counter-Strike 2 не найден.
    pause
    exit /b
)

echo CS2 найден: %cspath%
echo.

set "file=%cspath%\radar.dem"

if exist "%file%" (
    del "%file%"
    color 0A
    echo ══════════════════════════════════════
    echo   ✓ Файл radar.dem успешно удалён!
    echo ══════════════════════════════════════
) else (
    color 0E
    echo ══════════════════════════════════════
    echo   ✗ Файл radar.dem не найден.
    echo ══════════════════════════════════════
)

echo.
echo [V] найс, готово, закрывай
pause >nul