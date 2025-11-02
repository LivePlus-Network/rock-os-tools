# CLAUDE.md - AI Assistant Guidance for rock-os-tools

**Project**: rock-os-tools - Go-based orchestration tools for ROCK-OS
**Critical**: THIS PROJECT CREATES IMAGES THAT MUST BOOT WITH ROCK-INIT
**Language**: Go only (no Rust in this project)

---

## 🚨 CRITICAL: Integration Contract with Rock-Init

**THE MOST IMPORTANT RULE**: Rock-init has hardcoded paths. If you put files in the wrong place, ROCK-OS will not boot.

### Hardcoded Paths in Rock-Init (DO NOT CHANGE)
```go
// These constants MUST be in every relevant tool:
const (
    ROCK_INIT_PATH     = "/sbin/init"           // rock-init renamed to this
    ROCK_MANAGER_PATH  = "/usr/bin/rock-manager" // Line 553 in rock-init
    VOLCANO_AGENT_PATH = "/usr/bin/volcano-agent" // Lines 331, 581 in rock-init
    CONFIG_KEY_PATH    = "/config/CONFIG_KEY"     // Line 438 in rock-init
    BUSYBOX_PATH       = "/bin/busybox"
    SHELL_PATH         = "/bin/sh"               // Symlink to busybox
)
```

### Verification is MANDATORY
```bash
# After EVERY image creation:
../verify-rock-init-integration.sh image.cpio.gz
# MUST pass before considering the image valid
```

---

## 🎯 Project Overview

This project contains 10 focused tools that:
1. Solve immediate problems (work TODAY)
2. Follow Unix philosophy (do one thing well)
3. Will become libraries for rock-os-image-server (future)
4. MUST integrate perfectly with rock-init (non-negotiable)

### Tool List and Integration Requirements

| Tool | Purpose | Integration Requirement |
|------|---------|------------------------|
| rock-kernel | Manage kernels | Must set `init=/sbin/init` in cmdline |
| rock-deps | Scan dependencies | Must include ALL required .so files |
| rock-build | Build components | Must output correct binary names |
| rock-image | Create images | **MOST CRITICAL** - Must place files at EXACT paths |
| rock-config | Manage configs | Must write to `/config/` or `/etc/rock/` |
| rock-verify | Verify images | Must check ALL integration requirements |
| rock-compose | Run pipelines | Must include verification step |
| rock-security | Security ops | Must place KEY at `/config/CONFIG_KEY` |
| rock-cache | Cache artifacts | No direct integration requirements |
| rock-registry | Component registry | No direct integration requirements |

---

## 📋 Development Rules

### DO:
- ✅ **ALWAYS verify integration** after building images
- ✅ **Use exact paths** from the integration contract
- ✅ **Test with actual rock-init** in QEMU
- ✅ **Read integration specs** before implementing
- ✅ **Fail fast** if paths are wrong
- ✅ **Include integration tests** for each tool
- ✅ **Document any assumptions** about rock-init

### DO NOT:
- ❌ **Change paths** without updating rock-init
- ❌ **Assume paths** - use the documented contract
- ❌ **Skip verification** - it will fail in production
- ❌ **Create images** without the verification step
- ❌ **Rename binaries** incorrectly (rock-init → init, not rock-init)
- ❌ **Forget permissions** (init must be 755)

---

## 🔧 Tool Implementation Guidelines

### rock-kernel
```go
// MUST set correct init path
func GetKernelCmdline(mode string) string {
    // CRITICAL: Use init=, not rdinit=
    base := "init=/sbin/init"
    // NOT: "rdinit=/sbin/init" - this is wrong!
}
```

### rock-deps
```go
// MUST find all dependencies
func ScanDependencies(binary string) ([]string, error) {
    // Include musl libc if dynamically linked
    // Check for libssl.so.3, libcrypto.so.3 for volcano-agent
}
```

### rock-image (MOST CRITICAL)
```go
// THIS IS WHERE INTEGRATION HAPPENS
func CreateInitramfs(rootfs string, components Components) error {
    // CRITICAL: Exact paths or nothing works

    // rock-init MUST be renamed
    copyFile(components.RockInit, filepath.Join(rootfs, "sbin/init"))
    os.Chmod(filepath.Join(rootfs, "sbin/init"), 0755)

    // These paths are hardcoded in rock-init
    copyFile(components.RockManager, filepath.Join(rootfs, "usr/bin/rock-manager"))
    copyFile(components.VolcanoAgent, filepath.Join(rootfs, "usr/bin/volcano-agent"))

    // Shell is required
    copyFile(components.Busybox, filepath.Join(rootfs, "bin/busybox"))
    os.Symlink("busybox", filepath.Join(rootfs, "bin/sh"))

    // Create required directories
    for _, dir := range []string{"proc", "sys", "dev", "tmp", "run", "var/log"} {
        os.MkdirAll(filepath.Join(rootfs, dir), 0755)
    }

    // Create device nodes
    createDeviceNodes(rootfs)

    // ALWAYS VERIFY
    return verifyStructure(rootfs)
}
```

### rock-verify
```go
// MUST check integration requirements
func VerifyIntegration(imagePath string) error {
    // This tool encapsulates verify-rock-init-integration.sh logic

    required := []Check{
        {"/sbin/init", "rock-init must be here"},
        {"/usr/bin/rock-manager", "hardcoded in rock-init"},
        {"/usr/bin/volcano-agent", "hardcoded in rock-init"},
        {"/bin/busybox", "shell required"},
        {"/bin/sh", "symlink to busybox"},
    }

    for _, check := range required {
        if !exists(check.path) {
            return fmt.Errorf("INTEGRATION FAIL: %s - %s", check.path, check.reason)
        }
    }
}
```

---

## 🧪 Testing Requirements

### Every Tool Must Have Integration Tests
```go
func TestRockInitIntegration(t *testing.T) {
    // Build test image
    image := buildTestImage()

    // Verify structure
    err := VerifyIntegration(image)
    assert.NoError(t, err)

    // Actually boot in QEMU
    success := bootInQEMU(image)
    assert.True(t, success, "Must boot with rock-init")
}
```

### Pipeline Must Include Verification
```yaml
# Every pipeline.yaml must have:
stages:
  - name: create-image
    tool: rock-image
    args: ["cpio", "create", "./rootfs"]

  - name: verify-integration  # MANDATORY
    tool: rock-verify
    args: ["integration", "image.cpio.gz"]
    depends: ["create-image"]
```

---

## 📁 Project Structure

```
/Volumes/4TB/rock-os-tools/
├── CLAUDE.md              # THIS FILE (copy it there)
├── Makefile               # Build system
├── config.env            # Configuration
├── cmd/                  # Tool implementations
│   └── rock-image/       # MOST CRITICAL for integration
│       └── main.go
├── pkg/
│   └── integration/      # Shared integration code
│       ├── paths.go      # Path constants
│       ├── verify.go     # Verification logic
│       └── contract.go   # Integration contract
└── test/
    └── integration/      # Integration tests
        └── boot_test.go  # Actually boots in QEMU
```

---

## 🔗 Critical References

These documents are in `/Volumes/4TB/ROCK-MASTER/`:

1. **ROCK_INIT_INTEGRATION_SPEC.md** - Complete specification
2. **ROCK_INIT_INTEGRATION_UPDATE.md** - Based on code analysis
3. **verify-rock-init-integration.sh** - Verification script
4. **INTEGRATION_ANSWER.md** - Why this matters

**READ THESE BEFORE IMPLEMENTING ANY TOOL**

---

## 🚀 Starting Development

```bash
# 1. Setup project (if not done)
cd /Volumes/4TB/ROCK-MASTER/rock-os-tools-setup
./setup.sh

# 2. Change to project
cd /Volumes/4TB/rock-os-tools

# 3. Copy this CLAUDE.md
cp /Volumes/4TB/ROCK-MASTER/rock-os-tools-CLAUDE.md ./CLAUDE.md

# 4. Initialize Claude in this directory
claude init

# 5. Start development
make rock-kernel  # Start with kernel tool
```

---

## ⚠️ Common Integration Mistakes

### Mistake 1: Wrong Init Path
```go
// WRONG
cmdline := "rdinit=/sbin/init"  // rdinit is wrong!

// CORRECT
cmdline := "init=/sbin/init"    // Must be init=
```

### Mistake 2: Not Renaming rock-init
```go
// WRONG
copyFile("rock-init", "/sbin/rock-init")  // Wrong name!

// CORRECT
copyFile("rock-init", "/sbin/init")       // Must be renamed
```

### Mistake 3: Wrong Binary Locations
```go
// WRONG
copyFile("volcano-agent", "/bin/volcano-agent")        // Wrong directory!

// CORRECT
copyFile("volcano-agent", "/usr/bin/volcano-agent")    // Must be /usr/bin/
```

### Mistake 4: Missing Verification
```go
// WRONG
func CreateImage() {
    // ... create image ...
    return nil  // No verification!
}

// CORRECT
func CreateImage() {
    // ... create image ...
    return VerifyIntegration(image)  // ALWAYS verify
}
```

---

## 🎯 Success Criteria

Your tools are successful when:

1. ✅ `verify-rock-init-integration.sh` passes
2. ✅ Image boots in QEMU with rock-init
3. ✅ All services start (volcano-agent, rock-manager)
4. ✅ No hardcoded paths are wrong
5. ✅ Integration tests pass

---

## 📝 Quality Requirements

- **No placeholder code** in production paths
- **Full error handling** for all file operations
- **Integration tests** for every tool that touches paths
- **Documentation** of any rock-init assumptions
- **Verification** built into the workflow

---

## 🔐 The Integration Contract

This is sacred. Break it and nothing works:

```yaml
integration_contract:
  version: "1.0"

  binaries:
    - source: "rock-init"
      destination: "/sbin/init"
      permissions: 0755

    - source: "rock-manager"
      destination: "/usr/bin/rock-manager"
      permissions: 0755

    - source: "volcano-agent"
      destination: "/usr/bin/volcano-agent"
      permissions: 0755

    - source: "busybox"
      destination: "/bin/busybox"
      permissions: 0755
      symlinks: ["sh"]

  config:
    encryption_key: "/config/CONFIG_KEY"
    node_config: "/config/node.yaml"

  verification:
    required: true
    tool: "rock-verify"
    script: "../verify-rock-init-integration.sh"
```

---

## 🔢 MAC Address Management - rock-mac Tool

**Implementation follows rock-os-tools project structure:**
- Tool code: `cmd/rock-mac/`
- Shared database library: `pkg/mac/`
- Integration tests: `test/integration/mac/`
- Binary output: `bin/rock-mac`

### Overview

**rock-mac** is the centralized MAC address dispenser for ROCK OS nodes. It ensures:
- All MAC addresses use the ROCK OS OUI: `a4:58:0f`
- No duplicate MACs across all deployments
- Organized pools for different environments
- Full audit trail of allocations
- SQLite database for persistence

### Installation and Setup

```bash
# 1. Initialize database (first time only)
cd /Volumes/4TB/rock-os-tools
./scripts/init-mac-dispenser.sh

# 2. Build rock-mac tool (follows project structure)
cd cmd/rock-mac
go build -o ../../bin/rock-mac
# OR use the Makefile from project root:
cd /Volumes/4TB/rock-os-tools
make rock-mac

# 3. Install system-wide (optional)
sudo cp bin/rock-mac /usr/local/bin/

# 4. Verify installation
bin/rock-mac --version
bin/rock-mac stats
```

### Configuration

**Database Location**: `~/.rock/mac-dispenser.db`
**Config File**: `configs/mac-dispenser.yaml`

### Pools

| Pool | Range | Auto-Release | Usage |
|------|-------|--------------|-------|
| production | a4:58:0f:00:00:01 - a4:58:0f:00:ff:ff | Never | Production nodes |
| development | a4:58:0f:01:00:00 - a4:58:0f:01:ff:ff | 7 days | Dev/test nodes |
| experiment | a4:58:0f:02:00:00 - a4:58:0f:02:ff:ff | 1 day | Quick experiments |
| vultr | a4:58:0f:03:00:00 - a4:58:0f:03:ff:ff | 30 days | Vultr cloud VMs |
| docker | a4:58:0f:04:00:00 - a4:58:0f:04:ff:ff | 1 day | Docker containers |
| kubernetes | a4:58:0f:05:00:00 - a4:58:0f:05:ff:ff | 1 day | K8s pods |
| reserved | a4:58:0f:ff:00:00 - a4:58:0f:ff:ff:ff | Never | Special use |

### Usage

#### Allocate MAC Address

```bash
# Quick allocation (defaults to development pool)
rock-mac allocate

# Production allocation
rock-mac allocate --pool production --device prod-node-01 --type qemu-vm

# With metadata
rock-mac allocate --pool vultr --device vultr-01 --metadata '{"region":"nyc3","plan":"vc2-1c-1gb"}'
```

#### List Allocations

```bash
# List all active MACs
rock-mac list

# Filter by pool
rock-mac list --pool production

# Show all (including released)
rock-mac list --status all --limit 500
```

#### Release MAC Address

```bash
# Release by MAC address
rock-mac release a4:58:0f:00:00:01

# Release by device ID
rock-mac release dev-node-42

# Force release (even if reserved)
rock-mac release --force a4:58:0f:ff:00:01
```

#### Reserve Specific MAC

```bash
# Reserve specific MAC
rock-mac reserve --mac a4:58:0f:00:00:ff --device special-node

# Reserve next available in pool
rock-mac reserve --pool reserved --device monitoring-01
```

#### Show Statistics

```bash
# Pool statistics
rock-mac stats

# Detailed info for specific MAC
rock-mac show a4:58:0f:00:00:01
```

#### Cleanup Expired

```bash
# Auto-release expired allocations
rock-mac cleanup

# Dry run (show what would be released)
rock-mac cleanup --dry-run

# Override expiration (release after 3 days)
rock-mac cleanup --days 3
```

### Integration with Other Tools

#### rock-builder Integration

```bash
# rock-builder can request MAC automatically
MAC=$(rock-mac allocate --pool experiment --device "build-$(date +%s)" | grep "Allocated MAC:" | cut -d: -f2-)

# Use in image creation
rock-builder --mac $MAC ...
```

#### rock-image Integration

```go
// rock-image can call rock-mac programmatically
func GetMAC(pool, deviceID string) (string, error) {
    cmd := exec.Command("rock-mac", "allocate",
        "--pool", pool,
        "--device", deviceID)
    output, err := cmd.Output()
    // Parse MAC from output
    return parseMAC(output), err
}
```

#### Pipeline Integration

```yaml
# In pipeline.yaml
stages:
  - name: allocate-mac
    command: rock-mac allocate --pool ${POOL} --device ${NODE_ID}
    capture: MAC_ADDRESS

  - name: create-image
    tool: rock-image
    args: ["--mac", "${MAC_ADDRESS}"]
    depends: ["allocate-mac"]
```

### Database Schema

```sql
-- Main allocation table
mac_allocations (
    mac_address TEXT UNIQUE,  -- Full MAC (e.g., a4:58:0f:00:00:01)
    pool TEXT,                 -- Pool name
    device_id TEXT,            -- Device identifier
    device_type TEXT,          -- Type (qemu-vm, docker, etc.)
    metadata TEXT,             -- JSON metadata
    allocated_at TIMESTAMP,    -- When allocated
    released_at TIMESTAMP,     -- When released (if applicable)
    status TEXT                -- active, released, reserved
)

-- Audit trail
audit_log (
    action TEXT,               -- allocate, release, reserve, cleanup
    mac_address TEXT,          -- Affected MAC
    user TEXT,                 -- Who performed action
    timestamp TIMESTAMP        -- When it happened
)
```

### Maintenance

```bash
# Backup database
cp ~/.rock/mac-dispenser.db ~/.rock/mac-dispenser.db.backup

# Check database integrity
sqlite3 ~/.rock/mac-dispenser.db "PRAGMA integrity_check;"

# Export allocations
sqlite3 ~/.rock/mac-dispenser.db ".mode csv" ".headers on" \
  "SELECT * FROM mac_allocations WHERE status='active';" > active-macs.csv

# View recent activity
sqlite3 ~/.rock/mac-dispenser.db \
  "SELECT * FROM audit_log ORDER BY timestamp DESC LIMIT 20;"
```

### Troubleshooting

#### "Database not found"
```bash
# Initialize database
cd /Volumes/4TB/rock-os-tools
./scripts/init-mac-dispenser.sh
```

#### "MAC already allocated"
```bash
# Check who has it
rock-mac show a4:58:0f:XX:XX:XX

# Release if needed
rock-mac release a4:58:0f:XX:XX:XX
```

#### "Pool exhausted"
```bash
# Check pool usage
rock-mac stats

# Cleanup expired allocations
rock-mac cleanup

# Or use different pool
rock-mac allocate --pool development
```

### Critical Integration Points

1. **MAC Format**: Always `a4:58:0f:XX:XX:XX` (no exceptions)
2. **Database Path**: `~/.rock/mac-dispenser.db` (hardcoded in tools)
3. **Pool Names**: Must match configuration (case-sensitive)
4. **Auto-Release**: Runs via cron or manual cleanup
5. **Audit Log**: Never deleted (compliance requirement)

### Testing

```bash
# Run integration test
cd /Volumes/4TB/rock-os-tools
./test/mac-dispenser-test.sh

# Test database operations
sqlite3 ~/.rock/mac-dispenser.db < test/mac-queries.sql

# Verify pool ranges
rock-mac stats | grep -E "production|development|experiment"
```

---

**Remember: Get the paths wrong and rock-init won't boot. This is not optional.**