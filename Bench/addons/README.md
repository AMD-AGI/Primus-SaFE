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

Run scripts on multiple nodes via SSH in batch.

### Usage

```bash
./install.sh <nodes_file> <scripts_dir>
```

### Arguments

| Argument     | Description                                                                 |
|--------------|-----------------------------------------------------------------------------|
| nodes_file   | Node list file, one hostname per line (supports `#` comments and empty lines) |
| scripts_dir  | Scripts directory; all executable files are run in alphabetical order      |

### Prerequisites

- SSH key-based authentication configured (passwordless login)
- Scripts in scripts_dir must be executable (`chmod +x`)

### Behavior

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

# Run
./installinstall_on_nodes.sh nodes.txt scripts
```
