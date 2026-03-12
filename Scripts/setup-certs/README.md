# AMD CA Certificates Setup

One-command setup for trusting AMD internal services (e.g., SaFE platform, MCP servers). Installs the AMD Corporate Root CA and Issuing CA into the system trust store, and configures Node.js (used by Cursor/VS Code) to use them.

## Quick Start

### Linux (SaFE Authoring / Remote SSH)

No need to clone — just run:

```bash
# Root user
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.sh | bash

# Non-root user
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.sh | sudo bash
```

The script downloads the certificates from GitHub automatically.

Reconnect your Cursor Remote SSH session after running the script.

### Windows

Open PowerShell as Administrator and run:

```powershell
irm https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.bat -OutFile $env:TEMP\setup.bat; cmd /c $env:TEMP\setup.bat
```

Restart Cursor after running the script.

## What the Scripts Do

| Step | Linux (`setup.sh`) | Windows (`setup.bat`) |
|------|--------------------|-----------------------|
| 1 | Copies certs to `/usr/local/share/ca-certificates/` and runs `update-ca-certificates` | Imports Root CA to `Trusted Root Certification Authorities` and Issuing CA to `Intermediate Certification Authorities` via `certutil` |
| 2 | Adds `NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt` to `/etc/environment` | Creates `%USERPROFILE%\.amd-certs\amd-ca-chain.pem` and sets `NODE_EXTRA_CA_CERTS` via `setx` |

After setup, the following tools will trust AMD internal HTTPS services:

- `curl`, `wget`, `python requests`
- Node.js, Cursor, VS Code (MCP connections, extensions, etc.)

## Included Certificates

| File | Subject | Issuer | Expires |
|------|---------|--------|---------|
| `amd-root-ca.crt` | AMD Corporate Root CA | AMD Corporate Root CA (self-signed) | 2032-08-06 |
| `amd-issuing-ca.crt` | AMD-com Issuing CA | AMD Corporate Root CA | 2032-08-06 |

These are **public CA certificates** (no private keys). Safe to store in public repositories.
