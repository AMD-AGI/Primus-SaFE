# Addons Scripts

## sinfo_to_nodelist.sh

Parse `sinfo` output and expand NODELIST to one host per line. Handles SLURM bracket notation (e.g. `[021,042,079-080]` → `021`, `042`, `079`, `080`).

### Usage

```bash
./sinfo_to_nodelist.sh [output_file]
sinfo | ./sinfo_to_nodelist.sh [output_file]
```

### Example

```bash
# Output to stdout
./sinfo_to_nodelist.sh

# Output to file
./sinfo_to_nodelist.sh nodes.txt

# Pipe sinfo
sinfo | ./sinfo_to_nodelist.sh nodes.txt
```

---

## install.sh

Run scripts on multiple nodes via SSH in batch. Hosts are processed in parallel.

### Usage

```bash
./install.sh <nodes_file> <scripts_dir> [cluster_name]
```

### Arguments

| Argument      | Description                                                                 |
|---------------|-----------------------------------------------------------------------------|
| nodes_file    | Node list file, one hostname per line (supports `#` comments and empty lines) |
| scripts_dir   | Scripts directory; top-level scripts only (no subdirs) run in alphabetical order |
| cluster_name | Optional. If provided, additionally runs scripts from `scripts_dir/<cluster_name>/` |

### Prerequisites

- SSH key-based authentication configured (passwordless login)
- Scripts in scripts_dir must be executable (`chmod +x`)
- Python 3 for the installer

### Behavior

- Copies entire scripts directory (including subdirs) to each host via scp before running
- Each script runs with its directory as cwd, so scripts can call siblings (e.g. `bash other.sh`)
- Hosts are processed in parallel (up to 32 concurrent)
- SSH uses `StrictHostKeyChecking=no` to auto-accept new hosts
- On script failure, skip and continue with the next script
- Final output shows execution status (OK/FAIL) per node and per script

### Example

```bash
# Prepare node list
echo -e "node1\nnode2\nnode3" > nodes.txt

# Prepare scripts directory
mkdir -p scripts
echo '#!/bin/bash\necho "hello"' > scripts/01_hello.sh
chmod +x scripts/01_hello.sh

# Run top-level scripts only (no subdirs)
./install.sh nodes.txt scripts

# OCI cluster: run top-level scripts + scripts/oci/
./install.sh nodes.txt scripts oci
```
