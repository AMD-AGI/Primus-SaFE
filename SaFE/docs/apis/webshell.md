# WebShell API

## Overview
The WebShell API provides web-based terminal functionality for accessing workload containers directly through a browser. Built on WebSocket technology, it enables real-time, interactive shell sessions to running Pods without requiring SSH clients or VPN connections. The API supports multiple shell types (bash, sh, zsh), terminal resizing, and provides secure authenticated access with comprehensive audit logging. It serves as a convenient tool for development, debugging, and operational management of containerized workloads.

### Core Concepts

A WebShell session is a WebSocket-based terminal connection to a container, with the following key characteristics:

* WebSocket Protocol: Utilizes WebSocket for bidirectional, low-latency communication between browser and container shell.
* Shell Flexibility: Supports multiple shell types with automatic fallback to available shells in the container.
* Terminal Emulation: Provides full terminal emulation with support for resizing and standard terminal features.
* Secure Access: Requires authentication and authorization, with all sessions logged for audit and security compliance.

## API List

### 1. Establish WebShell Connection

Establish WebSocket connection to workload Pod.

**Endpoint**: `GET /api/v1/workloads/:name/pods/:podId/webshell`

**Protocol**: WebSocket

**Authentication Required**: Yes

**Path Parameters**:
- `name`: Workload ID
- `podId`: Pod ID

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| namespace | string | Yes | Kubernetes namespace where the Pod is located |
| container | string | No | Container name, default main container |
| cmd | string | No | Shell type: bash/sh/zsh, default sh |
| rows | int | No | Terminal rows, default 1800 |
| cols | int | No | Terminal columns, default 40 |

**WebSocket Upgrade Request Example**:
```
GET /api/v1/workloads/my-job-abc/pods/my-job-abc-worker-0/webshell?namespace=default&container=pytorch&cmd=bash&rows=40&cols=120
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
```

---

## WebSocket Protocol

### Message Format

WebShell uses standard WebSocket binary message format.

**Client Sends**:
- User input commands and characters
- Window size change events

**Server Sends**:
- Terminal output content
- Error messages

### Window Size Adjustment

When browser window size changes, client needs to send terminal size adjustment message:

```json
{
  "type": "resize",
  "rows": 40,
  "cols": 120
}
```

---

## Usage Examples

### JavaScript WebSocket Client

```javascript
// Establish WebSocket connection
const wsUrl = `wss://api.example.com/api/v1/workloads/my-job/pods/my-pod/webshell?namespace=default&container=main&cmd=bash`;
const ws = new WebSocket(wsUrl);

// Connection established
ws.onopen = () => {
  console.log('WebShell connected');
};

// Receive messages
ws.onmessage = (event) => {
  const data = event.data;
  // Display in terminal
  terminal.write(data);
};

// Send command
ws.send('ls -la\n');

// Send window size adjustment
ws.send(JSON.stringify({
  type: 'resize',
  rows: 40,
  cols: 120
}));

// Close connection
ws.close();
```

---

## Supported Shells

| Shell | Description |
|-------|-------------|
| bash | Bourne Again Shell (recommended) |
| sh | Bourne Shell |
| zsh | Z Shell |

System will try to start the specified shell type, falling back to available shells if not present.

---

## Security

### Authentication
- WebShell connections require valid authentication token
- Token can be passed via Cookie or URL parameter

### Authorization
- User must have access permission to the workload
- Can only access self-created workloads (unless administrator)

### Audit
- All WebShell sessions are logged
- Including connection time, user, executed commands, etc.

---

## Use Cases

### Development and Debugging
- View program output in real-time
- Interactive debugging
- File system operations

### Operations Management
- View log files
- Modify configuration files
- Check process status

---

## Limitations

### Connection Limits
- Maximum 10 concurrent WebShell connections per user
- Auto-disconnect after 30 minutes of inactivity

### Feature Limitations
- Does not support file upload/download (use volume mounts)
- Does not support X11 forwarding
- Some terminal features may not be supported

---

## Troubleshooting

### Connection Failed
1. Check if workload and Pod are running
2. Confirm container name is correct
3. Verify authentication token is valid

### Display Issues
1. Adjust terminal window size
2. Check character encoding settings (recommend UTF-8)
3. Try different shells

### Slow Response
1. Check network connection
2. Ensure Pod has sufficient resources
3. Check system load

---

## Notes

1. **Security Risk**: WebShell provides full shell access, use with caution
2. **Resource Consumption**: Many concurrent WebShell connections consume system resources
3. **Command Recording**: Executed commands are logged for audit purposes
4. **Session Persistence**: Closing browser disconnects WebShell connection
5. **Auto Reconnect**: Recommend implementing auto-reconnect logic in client
