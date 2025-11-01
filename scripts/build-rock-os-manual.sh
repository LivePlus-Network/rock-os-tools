#!/bin/bash
#
# build-rock-os-manual.sh - Manual step-by-step ROCK-OS build
# This script shows each command for educational purposes
#

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Configuration
BUILD_DIR="/tmp/rock-build"
ROOTFS_DIR="/tmp/rock-rootfs"
OUTPUT_DIR="$(pwd)/output"
BUILD_MODE="${1:-debug}"  # debug or production

# Helper function
step() {
    echo ""
    echo -e "${BLUE}==> $1${NC}"
}

error_exit() {
    echo -e "${RED}❌ Error: $1${NC}"
    exit 1
}

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo -e "${GREEN}====================================${NC}"
echo -e "${GREEN}   ROCK-OS Complete Build Pipeline  ${NC}"
echo -e "${GREEN}====================================${NC}"
echo ""
echo "Build mode: $BUILD_MODE"
echo "Output dir: $OUTPUT_DIR"
echo ""
echo "This will:"
echo "  1. Build all ROCK-OS components"
echo "  2. Create a bootable initramfs"
echo "  3. Verify integration with rock-init"
echo "  4. Prepare for QEMU testing"
echo ""
read -p "Continue? (y/N) " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 0
fi

# Step 1: Build components
step "Step 1: Building ROCK-OS components"
echo "Command: rock-build all --mode=$BUILD_MODE --output=$BUILD_DIR"
rock-build all --mode="$BUILD_MODE" --output="$BUILD_DIR" || error_exit "Build failed"

# Verify binaries exist
echo "Checking built binaries..."
ls -lh "$BUILD_DIR"/rock-init "$BUILD_DIR"/rock-manager "$BUILD_DIR"/volcano-agent

# Step 2: Prepare rootfs
step "Step 2: Preparing rootfs directory structure"
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"/{bin,sbin,usr/bin,etc/rock,config,proc,sys,dev,tmp,run,var/log,lib}
echo "✓ Created directory structure"

# Step 3: Install binaries (CRITICAL PATHS!)
step "Step 3: Installing binaries at rock-init expected paths"
echo -e "${YELLOW}CRITICAL: Using exact paths that rock-init expects!${NC}"

cp "$BUILD_DIR/rock-init" "$ROOTFS_DIR/sbin/init"
echo "  ✓ rock-init → /sbin/init"

cp "$BUILD_DIR/rock-manager" "$ROOTFS_DIR/usr/bin/rock-manager"
echo "  ✓ rock-manager → /usr/bin/rock-manager"

cp "$BUILD_DIR/volcano-agent" "$ROOTFS_DIR/usr/bin/volcano-agent"
echo "  ✓ volcano-agent → /usr/bin/volcano-agent"

chmod 755 "$ROOTFS_DIR"/sbin/init
chmod 755 "$ROOTFS_DIR"/usr/bin/rock-manager
chmod 755 "$ROOTFS_DIR"/usr/bin/volcano-agent

# Step 4: Install BusyBox
step "Step 4: Installing BusyBox"
BUSYBOX_PATH="${HOME}/.rock/cache/busybox-1.35.0"
if [ ! -f "$BUSYBOX_PATH" ]; then
    echo "Downloading BusyBox..."
    mkdir -p "$(dirname "$BUSYBOX_PATH")"
    curl -L -o "$BUSYBOX_PATH" \
        https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
    chmod 755 "$BUSYBOX_PATH"
fi
cp "$BUSYBOX_PATH" "$ROOTFS_DIR/bin/busybox"

# Create symlinks
cd "$ROOTFS_DIR/bin"
for cmd in sh ls cat ps echo mount umount sleep grep find; do
    ln -sf busybox "$cmd"
done
cd - > /dev/null
echo "✓ BusyBox installed with symlinks"

# Step 5: Scan and copy dependencies
step "Step 5: Scanning and copying dependencies"
echo "Command: rock-deps copy <binary> <lib-dir>"

rock-deps copy "$ROOTFS_DIR/sbin/init" "$ROOTFS_DIR/lib" || true
rock-deps copy "$ROOTFS_DIR/usr/bin/rock-manager" "$ROOTFS_DIR/lib" || true
rock-deps copy "$ROOTFS_DIR/usr/bin/volcano-agent" "$ROOTFS_DIR/lib" || true
echo "✓ Dependencies copied (if any)"

# Step 6: Generate configuration
step "Step 6: Generating configuration"
echo "Command: rock-config generate node"
rock-config generate node \
    --output="$ROOTFS_DIR/etc/rock/config.yaml" \
    --node-id=test-node \
    --mac=a4:58:0f:00:00:01 \
    --volcano=localhost:50061
echo "✓ Configuration generated"

# Step 7: Generate security keys
step "Step 7: Generating security keys (optional)"
if [[ "$BUILD_MODE" == "production" ]]; then
    echo "Command: rock-security keygen"
    rock-security keygen --output="$ROOTFS_DIR/config/CONFIG_KEY"
    echo "✓ Encryption key generated"
else
    echo "Skipping in debug mode"
fi

# Step 8: Create device nodes
step "Step 8: Creating device nodes"
cd "$ROOTFS_DIR/dev"
sudo mknod -m 666 null c 1 3 2>/dev/null || true
sudo mknod -m 666 zero c 1 5 2>/dev/null || true
sudo mknod -m 666 random c 1 8 2>/dev/null || true
sudo mknod -m 666 urandom c 1 9 2>/dev/null || true
sudo mknod -m 666 tty c 5 0 2>/dev/null || true
sudo mknod -m 666 console c 5 1 2>/dev/null || true
cd - > /dev/null
echo "✓ Device nodes created"

# Step 9: Build initramfs
step "Step 9: Creating initramfs image"
echo "Command: rock-image cpio create $ROOTFS_DIR"
rock-image cpio create "$ROOTFS_DIR" \
    --output="$OUTPUT_DIR/rock-os.cpio.gz" \
    --compress=gzip
echo "✓ Image created: $OUTPUT_DIR/rock-os.cpio.gz"
ls -lh "$OUTPUT_DIR/rock-os.cpio.gz"

# Step 10-12: Verify everything
step "Step 10: Verifying rock-init integration"
echo "Command: rock-verify integration"
rock-verify integration "$OUTPUT_DIR/rock-os.cpio.gz" || error_exit "Integration verification failed!"

step "Step 11: Verifying structure"
echo "Command: rock-verify structure"
rock-verify structure "$OUTPUT_DIR/rock-os.cpio.gz"

step "Step 12: Verifying dependencies"
echo "Command: rock-verify dependencies"
rock-verify dependencies "$OUTPUT_DIR/rock-os.cpio.gz"

# Step 13-14: Get kernel
step "Step 13: Fetching kernel"
echo "Command: rock-kernel fetch alpine:5.10.180"
rock-kernel fetch alpine:5.10.180

step "Step 14: Extracting kernel"
KERNEL_APK="${HOME}/.rock/kernels/alpine-5.10.180.apk"
if [ -f "$KERNEL_APK" ]; then
    rock-kernel extract "$KERNEL_APK"
    cp "${HOME}/.rock/kernels/vmlinuz" "$OUTPUT_DIR/vmlinuz"
    echo "✓ Kernel extracted to: $OUTPUT_DIR/vmlinuz"
else
    error_exit "Kernel APK not found"
fi

# Step 15: Cache the build
step "Step 15: Caching build artifacts"
echo "Command: rock-cache store"
rock-cache store "rock-os-$BUILD_MODE" "$OUTPUT_DIR/rock-os.cpio.gz"
echo "✓ Build cached"

# Final summary
echo ""
echo -e "${GREEN}====================================${NC}"
echo -e "${GREEN}      BUILD SUCCESSFUL!             ${NC}"
echo -e "${GREEN}====================================${NC}"
echo ""
echo "Output files:"
ls -lh "$OUTPUT_DIR"/*.cpio.gz "$OUTPUT_DIR"/vmlinuz
echo ""
echo -e "${GREEN}Step 7: Test in QEMU${NC}"
echo ""
echo "To test your build in QEMU, run:"
echo ""
echo -e "${YELLOW}  chmod +x scripts/test-boot.sh${NC}"
echo -e "${YELLOW}  ./scripts/test-boot.sh $OUTPUT_DIR/vmlinuz $OUTPUT_DIR/rock-os.cpio.gz${NC}"
echo ""
echo "Or manually with:"
echo ""
echo -e "${YELLOW}  qemu-system-x86_64 \\${NC}"
echo -e "${YELLOW}    -m 512 \\${NC}"
echo -e "${YELLOW}    -kernel $OUTPUT_DIR/vmlinuz \\${NC}"
echo -e "${YELLOW}    -initrd $OUTPUT_DIR/rock-os.cpio.gz \\${NC}"
echo -e "${YELLOW}    -append \"init=/sbin/init console=ttyS0 debug\" \\${NC}"
echo -e "${YELLOW}    -nographic -serial mon:stdio${NC}"
echo ""
echo "Press Ctrl-A X to exit QEMU"