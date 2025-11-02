# rock-mac - ROCK OS MAC Address Dispenser

Centralized MAC address management for ROCK OS deployments.

## Features

- **Guaranteed Unique MACs**: Sequential allocation prevents duplicates
- **Pool-Based Organization**: 7 pools for different environments
- **ROCK OS OUI**: All MACs use `a4:58:0f` prefix
- **Auto-Release**: Configurable expiration per pool
- **Full Audit Trail**: Track all allocations and releases
- **SQLite Database**: Persistent, queryable storage
- **CLI Interface**: Simple commands for all operations

## Quick Start

```bash
# Initialize database (first time only)
../scripts/init-mac-dispenser.sh

# Build the tool
make deps
make build

# Allocate your first MAC
./build/rock-mac allocate
```

## Installation

### From Source

```bash
cd /Volumes/4TB/rock-os-tools/rock-mac
make deps
make build
make install  # Installs to /usr/local/bin
```

### Database Setup

The database is automatically initialized by the setup script:
```bash
/Volumes/4TB/rock-os-tools/scripts/init-mac-dispenser.sh
```

This creates:
- Database at `~/.rock/mac-dispenser.db`
- Test script at `~/.rock/test-mac-dispenser.sh`
- All required tables and views

## Usage Examples

### Basic Operations

```bash
# Allocate a MAC (defaults to development pool)
rock-mac allocate

# Allocate for production
rock-mac allocate --pool production --device prod-01

# List active allocations
rock-mac list

# Show statistics
rock-mac stats

# Release a MAC
rock-mac release a4:58:0f:00:00:01

# Get help
rock-mac --help
rock-mac allocate --help
```

### Advanced Usage

```bash
# Reserve a specific MAC
rock-mac reserve --mac a4:58:0f:00:00:ff --device special

# Cleanup expired allocations
rock-mac cleanup

# Show details for specific MAC
rock-mac show a4:58:0f:00:00:01

# List with filters
rock-mac list --pool production --status active --limit 50
```

## Pools

| Pool | Range | Auto-Release | Purpose |
|------|-------|--------------|---------|
| **production** | `00:00:01` - `00:ff:ff` | Never | Production systems |
| **development** | `01:00:00` - `01:ff:ff` | 7 days | Development/testing |
| **experiment** | `02:00:00` - `02:ff:ff` | 1 day | Quick experiments |
| **vultr** | `03:00:00` - `03:ff:ff` | 30 days | Vultr cloud VMs |
| **docker** | `04:00:00` - `04:ff:ff` | 1 day | Docker containers |
| **kubernetes** | `05:00:00` - `05:ff:ff` | 1 day | Kubernetes pods |
| **reserved** | `ff:00:00` - `ff:ff:ff` | Never | Special purposes |

All ranges use the `a4:58:0f` prefix.

## Integration

### With rock-builder

```bash
# Get MAC and use in image creation
MAC=$(rock-mac allocate --pool experiment --device test-01 | grep "Allocated" | awk '{print $3}')
rock-builder --mac $MAC create image.cpio.gz
```

### In Scripts

```bash
#!/bin/bash
# Allocate MAC for new VM
MAC=$(rock-mac allocate --pool development --device "vm-$(date +%s)" | awk '/Allocated MAC:/ {print $3}')

# Create VM with allocated MAC
qemu-system-x86_64 \
  -netdev user,id=net0 \
  -device e1000,netdev=net0,mac=$MAC \
  ...

# Release when done
trap "rock-mac release $MAC" EXIT
```

### From Go Code

```go
import "os/exec"

func allocateMAC(pool, device string) (string, error) {
    cmd := exec.Command("rock-mac", "allocate",
        "--pool", pool,
        "--device", device)
    output, err := cmd.Output()
    if err != nil {
        return "", err
    }
    // Parse MAC from output
    // Output format: "Allocated MAC: a4:58:0f:XX:XX:XX\n"
    mac := strings.TrimPrefix(string(output), "Allocated MAC: ")
    return strings.TrimSpace(mac), nil
}
```

## Database

### Location
- **Primary**: `~/.rock/mac-dispenser.db`
- **Backup**: Created automatically before writes

### Direct Access
```bash
# Open database
sqlite3 ~/.rock/mac-dispenser.db

# Useful queries
.tables                                    # List all tables
SELECT * FROM pool_stats;                  # View statistics
SELECT * FROM active_allocations LIMIT 10; # Recent allocations
SELECT * FROM audit_log ORDER BY timestamp DESC LIMIT 20; # Audit trail
```

### Backup & Restore
```bash
# Backup
cp ~/.rock/mac-dispenser.db ~/.rock/mac-dispenser.db.backup

# Restore
cp ~/.rock/mac-dispenser.db.backup ~/.rock/mac-dispenser.db
```

## Development

### Project Structure
```
rock-mac/
├── main.go          # CLI commands and structure
├── database.go      # Database operations
├── go.mod          # Go module definition
├── go.sum          # Dependency versions
├── Makefile        # Build automation
└── README.md       # This file
```

### Building
```bash
make deps    # Download dependencies
make build   # Build binary to ./build/
make test    # Run tests
make clean   # Remove build artifacts
```

### Testing
```bash
# Unit tests
go test -v ./...

# Integration test
make build
./build/rock-mac init
./build/rock-mac allocate --pool test
./build/rock-mac stats
```

## Configuration

Configuration file: `/Volumes/4TB/rock-os-tools/configs/mac-dispenser.yaml`

```yaml
mac_dispenser:
  prefix: "a4:58:0f"  # ROCK-OS OUI
  database:
    path: "~/.rock/mac-dispenser.db"
  pools:
    production:
      range: "00:00:01-00:ff:ff"
      auto_release_days: 0
    development:
      range: "01:00:00-01:ff:ff"
      auto_release_days: 7
    # ... more pools
```

## Troubleshooting

### Database Not Found
```bash
# Initialize database
cd /Volumes/4TB/rock-os-tools
./scripts/init-mac-dispenser.sh
```

### Build Errors
```bash
# Update dependencies
go mod tidy
go mod download

# Clean and rebuild
make clean
make deps
make build
```

### MAC Already Allocated
```bash
# Check allocation
rock-mac show a4:58:0f:XX:XX:XX

# Force release if needed
rock-mac release --force a4:58:0f:XX:XX:XX
```

## License

Part of the ROCK OS project. See main project for license details.

## Support

For issues or questions:
- Check CLAUDE.md in rock-os-tools
- Review database with `sqlite3 ~/.rock/mac-dispenser.db`
- Run test script: `~/.rock/test-mac-dispenser.sh`