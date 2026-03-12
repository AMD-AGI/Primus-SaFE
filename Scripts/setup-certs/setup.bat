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

echo [4/4] Configuring Node.js to use the CA certificates...
copy /b "%CERT_DIR%\amd-root-ca.crt"+"%CERT_DIR%\amd-issuing-ca.crt" "%CERT_DIR%\amd-ca-chain.pem" >nul
setx NODE_EXTRA_CA_CERTS "%CERT_DIR%\amd-ca-chain.pem"

echo.
echo Done! curl, Node.js (Cursor) will now trust AMD internal services.
echo Restart Cursor to pick up the new environment variable.

endlocal
