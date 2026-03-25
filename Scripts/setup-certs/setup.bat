@echo off
setlocal

set "REPO_RAW=https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs"
set "CERT_DIR=%USERPROFILE%\.amd-certs"

echo [1/4] Downloading CA certificates from GitHub...
if not exist "%CERT_DIR%" mkdir "%CERT_DIR%"
curl -fsSL "%REPO_RAW%/amd-root-ca.crt"    -o "%CERT_DIR%\amd-root-ca.crt"
curl -fsSL "%REPO_RAW%/amd-issuing-ca.crt" -o "%CERT_DIR%\amd-issuing-ca.crt"

echo [2/4] Installing AMD Root CA to Windows trust store...
certutil -addstore -f "Root" "%CERT_DIR%\amd-root-ca.crt"
if %ERRORLEVEL% neq 0 (
    echo ERROR: Failed to install Root CA. Run this script as Administrator.
    exit /b 1
)

echo [3/4] Installing AMD Issuing CA to Windows trust store...
certutil -addstore -f "CA" "%CERT_DIR%\amd-issuing-ca.crt"
if %ERRORLEVEL% neq 0 (
    echo ERROR: Failed to install Issuing CA. Run this script as Administrator.
    exit /b 1
)

echo [4/6] Configuring Node.js to use the CA certificates...
copy /b "%CERT_DIR%\amd-root-ca.crt"+"%CERT_DIR%\amd-issuing-ca.crt" "%CERT_DIR%\amd-ca-chain.pem" >nul
setx NODE_EXTRA_CA_CERTS "%CERT_DIR%\amd-ca-chain.pem"

echo [5/6] Appending CA certs to Python certifi bundle...
where python >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo   Python not found, skipping.
    goto :skip_python
)
for /f "delims=" %%B in ('python -c "import certifi; print(certifi.where())" 2^>nul') do set "BUNDLE=%%B"
if not defined BUNDLE (
    echo   Python certifi not found, skipping.
    goto :skip_python
)
findstr /c:"AMD Corporate Root CA" "%BUNDLE%" >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo   certifi bundle already contains AMD certs, skipping.
    goto :skip_python
)
type "%CERT_DIR%\amd-root-ca.crt" >> "%BUNDLE%"
type "%CERT_DIR%\amd-issuing-ca.crt" >> "%BUNDLE%"
echo   Appended AMD certs to %BUNDLE%
:skip_python

echo [6/6] Done!
echo.
echo   curl, python (requests/httpx), Node.js (Cursor) will now trust AMD internal services.
echo   Restart Cursor to pick up the new environment variable.

endlocal
