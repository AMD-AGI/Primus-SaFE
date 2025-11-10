# System Tuner

## Overview

System Tuner is a lightweight daemon service designed to automatically monitor and optimize critical Linux kernel parameters for containerized environments. It runs continuously to ensure that system-level configurations meet the minimum requirements for running resource-intensive applications like Elasticsearch, high-performance databases, and large-scale distributed systems.

## Features

- **Automatic VM Configuration**: Monitors and adjusts `vm.max_map_count` to ensure sufficient memory map areas
- **File Descriptor Limits**: Manages system-wide file descriptor limits (nofile) for optimal I/O operations
- **Continuous Monitoring**: Periodically checks system parameters every 30 seconds
- **Idempotent Operations**: Only applies changes when current values fall below thresholds
- **Container-Aware**: Designed to run in privileged containers with host system access

## System Requirements

- **Operating System**: Linux-based systems
- **Go Version**: 1.24 or higher
- **Privileges**: Must run with elevated privileges (root or equivalent)
- **Container Requirements**: 
  - Access to `/host-proc/sys/vm/` for reading kernel parameters
  - Access to `/etc/sysctl.conf` for persistent configuration
  - Access to `/etc/security/limits.conf` for file limits
  - Capability to execute `nsenter` command for applying sysctl changes

## Configuration

### Default Thresholds

| Parameter | Threshold Value | Purpose |
|-----------|----------------|---------|
| `vm.max_map_count` | 262144 | Maximum number of memory map areas a process may have |
| `nofile` (soft/hard) | 131072 | Maximum number of open file descriptors |

### Check Interval

- **Default**: 30 seconds
- The service continuously monitors and adjusts parameters at this interval

## How It Works

### VM Max Map Count Tuning

1. Reads the current `vm.max_map_count` value from `/host-proc/sys/vm/max_map_count`
2. Compares against the threshold (262144)
3. If below threshold:
   - Updates or adds the configuration in `/etc/sysctl.conf`
   - Applies changes using `nsenter` to execute `sysctl -p` in the host namespace
   - Ensures persistence across system reboots

### File Descriptor Limits Tuning

1. Reads the current settings from `/etc/security/limits.conf`
2. Checks both soft and hard limits for `nofile`
3. If below threshold (131072):
   - Updates existing entries or adds new ones
   - Applies to all users (`*` wildcard)
   - Changes take effect for new login sessions

## Building

```bash
cd cmd/system-tuner
go build -o system-tuner main.go
```

## Usage

### Running Directly

```bash
sudo ./system-tuner
```

### Running in Docker/Kubernetes

The service is designed to run as a DaemonSet or privileged container with the following requirements:

```yaml
securityContext:
  privileged: true
volumeMounts:
  - name: host-proc
    mountPath: /host-proc
    readOnly: true
  - name: etc-sysctl
    mountPath: /etc/sysctl.conf
  - name: etc-limits
    mountPath: /etc/security/limits.conf
```

## Output

The service provides detailed logging for all operations:

```
System-Tuner v0.1
Current vm.max_map_count: 65530
Executing sysctl -p to apply changes
vm.max_map_count set to 262144
Updated soft in /etc/security/limits.conf to 131072
Updated hard in /etc/security/limits.conf to 131072
```

## Why These Parameters Matter

### vm.max_map_count

- **Purpose**: Limits the number of memory-mapped areas a process can have
- **Impact**: Applications like Elasticsearch require high values to handle large indices
- **Symptoms of Low Value**: "max virtual memory areas vm.max_map_count is too low" errors

### nofile Limits

- **Purpose**: Controls the maximum number of file descriptors a process can open
- **Impact**: Affects database connections, network sockets, and file operations
- **Symptoms of Low Value**: "Too many open files" errors in logs

## Security Considerations

This tool requires privileged access to modify system-level configurations:

- Runs with root privileges
- Modifies system-wide configuration files
- Uses `nsenter` to access host namespace
- Should only be deployed in trusted environments

## Troubleshooting

### Changes Not Applied

- Verify the container has sufficient privileges
- Check volume mounts for configuration files
- Ensure `nsenter` is available in the container

### Parameters Reset After Reboot

- Verify `/etc/sysctl.conf` is correctly mounted and writable
- Check if another process or configuration management tool is overwriting settings

### Permission Denied Errors

- Confirm the container is running with `privileged: true`
- Verify SELinux/AppArmor policies allow the required operations

## Module Information

- **Module Path**: `github.com/AMD-AGI/primus-lens/system-tuner`
- **Go Version**: 1.24
- **Version**: 0.1

## License

This module is part of the Primus-SaFE project.

