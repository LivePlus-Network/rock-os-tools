#!/bin/bash
#
# build-with-rock-master.sh - Build using existing ROCK-MASTER scripts
# Since rock-build is not implemented yet, use the real scripts
#

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Configuration
ROCK_MASTER="/Volumes/4TB/ROCK-MASTER"
BUILD_DIR="/tmp/rock-build"
ROOTFS_DIR="/tmp/rock-rootfs"
OUTPUT_DIR="$(pwd)/output"
BUILD_MODE="${1:-debug}"

# Ensure we're in rock-os-tools directory
if [ ! -f "CLAUDE.md" ]; then
    echo -e "${RED}Error: Must run from rock-os-tools directory${NC}"
    exit 1
fi

echo -e "${GREEN}=====================================${NC}"
echo -e "${GREEN}   Building with ROCK-MASTER Scripts ${NC}"
echo -e "${GREEN}=====================================${NC}"
echo ""

# Step 1: Build components using ROCK-MASTER scripts
echo -e "${BLUE}Step 1: Building ROCK-OS components${NC}"
echo "Using ROCK-MASTER build scripts..."

mkdir -p "$BUILD_DIR"
cd "$ROCK_MASTER"

# Build rock-init
echo -e "${YELLOW}Building rock-init...${NC}"
./scripts/04_build_init.sh || echo -e "${RED}rock-init build failed${NC}"

# Build rock-manager
echo -e "${YELLOW}Building rock-manager...${NC}"
./scripts/05_build_manager.sh || echo -e "${RED}rock-manager build failed${NC}"

# Build volcano-agent
echo -e "${YELLOW}Building volcano-agent...${NC}"
./scripts/06_build_agent.sh || echo -e "${RED}volcano-agent build failed${NC}"

# Find the built binaries
echo -e "${BLUE}Looking for built binaries...${NC}"

# Direct paths based on build output
INIT_BINARY="$ROCK_MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-init"
MANAGER_BINARY="$ROCK_MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-manager"
AGENT_BINARY="$ROCK_MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/volcano-agent"

if [ ! -f "$INIT_BINARY" ]; then
    echo -e "${RED}rock-init binary not found at: $INIT_BINARY${NC}"
    exit 1
fi

if [ ! -f "$MANAGER_BINARY" ]; then
    echo -e "${RED}rock-manager binary not found at: $MANAGER_BINARY${NC}"
    exit 1
fi

if [ ! -f "$AGENT_BINARY" ]; then
    echo -e "${RED}volcano-agent binary not found at: $AGENT_BINARY${NC}"
    exit 1
fi

echo "Found binaries:"
echo "  rock-init: $INIT_BINARY"
echo "  rock-manager: $MANAGER_BINARY"
echo "  volcano-agent: $AGENT_BINARY"

# Copy to build directory
cp "$INIT_BINARY" "$BUILD_DIR/rock-init"
cp "$MANAGER_BINARY" "$BUILD_DIR/rock-manager"
cp "$AGENT_BINARY" "$BUILD_DIR/volcano-agent"

echo -e "${GREEN}✓ Binaries copied to $BUILD_DIR${NC}"

# Return to rock-os-tools
cd - > /dev/null

# Now continue with the rest of the pipeline
echo ""
echo -e "${BLUE}Step 2: Creating initramfs with rock-image${NC}"

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Prepare rootfs
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"/{bin,sbin,usr/bin,etc/rock,config,proc,sys,dev,tmp,run,var/log,lib}

# Install binaries at EXACT paths
echo -e "${YELLOW}Installing binaries at rock-init expected paths...${NC}"
cp "$BUILD_DIR/rock-init" "$ROOTFS_DIR/sbin/init"
cp "$BUILD_DIR/rock-manager" "$ROOTFS_DIR/usr/bin/rock-manager"
cp "$BUILD_DIR/volcano-agent" "$ROOTFS_DIR/usr/bin/volcano-agent"
chmod 755 "$ROOTFS_DIR"/sbin/init
chmod 755 "$ROOTFS_DIR"/usr/bin/*

echo "  ✓ /sbin/init (rock-init renamed)"
echo "  ✓ /usr/bin/rock-manager"
echo "  ✓ /usr/bin/volcano-agent"

# Install BusyBox
echo -e "${BLUE}Installing BusyBox...${NC}"
BUSYBOX_PATH="${HOME}/.rock/cache/busybox-1.35.0"
if [ ! -f "$BUSYBOX_PATH" ]; then
    mkdir -p "$(dirname "$BUSYBOX_PATH")"
    curl -L -o "$BUSYBOX_PATH" \
        https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
    chmod 755 "$BUSYBOX_PATH"
fi
cp "$BUSYBOX_PATH" "$ROOTFS_DIR/bin/busybox"
cd "$ROOTFS_DIR/bin"
for cmd in sh ls cat ps echo mount umount; do
    ln -sf busybox "$cmd"
done
cd - > /dev/null
echo "✓ BusyBox installed"

# Generate config
echo -e "${BLUE}Generating configuration...${NC}"
export PATH="$PATH:/Volumes/4TB/ROCK-MASTER/bin/tools"
rock-config generate node \
    --output="$ROOTFS_DIR/etc/rock/config.yaml" \
    --node-id=test-node \
    --mac=a4:58:0f:00:00:01 \
    --volcano=localhost:50061 || echo "config generation failed"

# Create device nodes
echo -e "${BLUE}Creating device nodes...${NC}"
cd "$ROOTFS_DIR/dev"
sudo mknod -m 666 null c 1 3 2>/dev/null || true
sudo mknod -m 666 zero c 1 5 2>/dev/null || true
sudo mknod -m 666 random c 1 8 2>/dev/null || true
sudo mknod -m 666 urandom c 1 9 2>/dev/null || true
cd - > /dev/null

# Build image
echo -e "${BLUE}Creating initramfs...${NC}"
rock-image cpio create "$ROOTFS_DIR" \
    --output="$OUTPUT_DIR/rock-os.cpio.gz" \
    --compress=gzip

ls -lh "$OUTPUT_DIR/rock-os.cpio.gz"

# Verify
echo -e "${BLUE}Verifying integration...${NC}"
rock-verify integration "$OUTPUT_DIR/rock-os.cpio.gz"

# Get kernel
echo -e "${BLUE}Getting kernel...${NC}"
if [ ! -f "${HOME}/.rock/kernels/vmlinuz" ]; then
    rock-kernel fetch alpine:5.10.180
    rock-kernel extract "${HOME}/.rock/kernels/alpine-5.10.180.apk"
fi
cp "${HOME}/.rock/kernels/vmlinuz" "$OUTPUT_DIR/vmlinuz"

echo ""
echo -e "${GREEN}=====================================${NC}"
echo -e "${GREEN}        BUILD COMPLETE!              ${NC}"
echo -e "${GREEN}=====================================${NC}"
echo ""
echo "Output files:"
ls -lh "$OUTPUT_DIR"
echo ""
echo "Test with:"
echo "  ./scripts/test-boot.sh $OUTPUT_DIR/vmlinuz $OUTPUT_DIR/rock-os.cpio.gz"