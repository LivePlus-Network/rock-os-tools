# ROCK-OS Tools

✅ **STATUS: FULLY FUNCTIONAL** - Pipeline successfully builds and boots ROCK-OS with rock-init as PID 1!

A suite of Go-based tools for building and managing ROCK-OS images.

## Quick Start

```bash
# Build and install all tools
make build && make install

# Add to PATH
export PATH="$PATH:/Volumes/4TB/ROCK-MASTER/bin/tools"

# Run the complete pipeline (builds bootable ROCK-OS image)
./scripts/quick-test.sh

# Test boot in QEMU
./scripts/test-boot.sh output/vmlinuz output/rock-os-final.cpio.gz
```

## Working Pipeline

The pipeline creates a bootable ROCK-OS image that successfully runs rock-init as PID 1:

```bash
# Full pipeline with all steps
rock-compose run pipelines/build-rock-os.yaml

# Or use the quick test script
./scripts/quick-test.sh
```

### Critical Boot Parameter
**IMPORTANT**: Use `rdinit=/sbin/init` (not `init=/sbin/init`) for initramfs-based systems

## Tools

- **rock-kernel** - Alpine Linux kernel management
- **rock-deps** - Dependency scanning and resolution
- **rock-build** - Component build orchestration
- **rock-image** - CPIO/ISO image creation
- **rock-config** - Configuration management
- **rock-compose** - Pipeline composition
- **rock-security** - Security operations
- **rock-cache** - Cache management
- **rock-verify** - Image verification
- **rock-registry** - Component registry

## Development

See docs/ for detailed documentation.
