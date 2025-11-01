# ROCK-OS Tools

A suite of Go-based tools for building and managing ROCK-OS images.

## Quick Start

```bash
# Build all tools
make build

# Install to ROCK-MASTER
make install

# Add to PATH
export PATH="$PATH:/Volumes/4TB/ROCK-MASTER/bin/tools"

# Use tools
rock-kernel fetch alpine:5.10.186
rock-deps scan ./experiment
rock-build all --mode debug
```

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
