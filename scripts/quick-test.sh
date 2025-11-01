#!/bin/bash
#
# quick-test.sh - Quick test using existing binaries
#

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

ROCK_MASTER="/Volumes/4TB/ROCK-MASTER"
ROOTFS_DIR="/tmp/rock-rootfs"
OUTPUT_DIR="$(pwd)/output"

export PATH="$PATH:/Volumes/4TB/ROCK-MASTER/bin/tools"

echo -e "${GREEN}Quick Test - Using Existing Binaries${NC}"
echo ""

# Step 1: Check existing binaries
echo -e "${BLUE}Step 1: Checking existing binaries${NC}"
INIT_BINARY="$ROCK_MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-init"
MANAGER_BINARY="$ROCK_MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-manager"
AGENT_BINARY="$ROCK_MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/volcano-agent"

ls -lh "$INIT_BINARY" | awk '{print "rock-init: " $5 " (modified: " $6 " " $7 " " $8 ")"}'
ls -lh "$MANAGER_BINARY" | awk '{print "rock-manager: " $5 " (modified: " $6 " " $7 " " $8 ")"}'
ls -lh "$AGENT_BINARY" | awk '{print "volcano-agent: " $5 " (modified: " $6 " " $7 " " $8 ")"}'

# Step 2: Create rootfs
echo ""
echo -e "${BLUE}Step 2: Creating rootfs${NC}"
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"/{bin,sbin,usr/bin,etc/rock,config,proc,sys,dev,tmp,run,var/log,lib}

# Step 3: Install binaries at EXACT paths
echo -e "${BLUE}Step 3: Installing binaries${NC}"
echo -e "${YELLOW}CRITICAL: Using exact paths for rock-init${NC}"
cp "$INIT_BINARY" "$ROOTFS_DIR/sbin/init"
cp "$MANAGER_BINARY" "$ROOTFS_DIR/usr/bin/rock-manager"
cp "$AGENT_BINARY" "$ROOTFS_DIR/usr/bin/volcano-agent"
chmod 755 "$ROOTFS_DIR"/sbin/init "$ROOTFS_DIR"/usr/bin/*
echo "  ✓ /sbin/init"
echo "  ✓ /usr/bin/rock-manager"
echo "  ✓ /usr/bin/volcano-agent"

# Step 4: Install BusyBox
echo ""
echo -e "${BLUE}Step 4: Installing BusyBox${NC}"
BUSYBOX_PATH="${HOME}/.rock/cache/busybox-1.35.0"
if [ ! -f "$BUSYBOX_PATH" ]; then
    mkdir -p "$(dirname "$BUSYBOX_PATH")"
    echo "Downloading BusyBox..."
    curl -L -o "$BUSYBOX_PATH" \
        https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
    chmod 755 "$BUSYBOX_PATH"
fi
cp "$BUSYBOX_PATH" "$ROOTFS_DIR/bin/busybox"
cd "$ROOTFS_DIR/bin"
for cmd in sh ls cat ps echo mount umount grep find; do
    ln -sf busybox "$cmd"
done
cd - > /dev/null
echo "✓ BusyBox installed with symlinks"

# Step 5: Create device nodes
echo ""
echo -e "${BLUE}Step 5: Creating device nodes${NC}"
cd "$ROOTFS_DIR/dev"
sudo mknod -m 666 null c 1 3 2>/dev/null || true
sudo mknod -m 666 zero c 1 5 2>/dev/null || true
sudo mknod -m 666 random c 1 8 2>/dev/null || true
sudo mknod -m 666 urandom c 1 9 2>/dev/null || true
sudo mknod -m 666 tty c 5 0 2>/dev/null || true
sudo mknod -m 666 console c 5 1 2>/dev/null || true
cd - > /dev/null
echo "✓ Device nodes created"

# Step 6: Create basic config
echo ""
echo -e "${BLUE}Step 6: Creating basic config${NC}"
cat > "$ROOTFS_DIR/etc/rock/config.yaml" << EOF
node:
  id: test-node
  mac: a4:58:0f:00:00:01
  labels:
    environment: test
    region: local

volcano:
  server: localhost:50061
  reconnect_interval: 5s

logging:
  level: debug
EOF
echo "✓ Basic config created"

# Step 7: Build image with rock-image
echo ""
echo -e "${BLUE}Step 7: Creating initramfs with rock-image${NC}"
mkdir -p "$OUTPUT_DIR"
rock-image cpio create "$ROOTFS_DIR" \
    --output="$OUTPUT_DIR/rock-os.cpio.gz" \
    --compress=gzip

if [ -f "$OUTPUT_DIR/rock-os.cpio.gz" ]; then
    echo -e "${GREEN}✓ Image created successfully${NC}"
    ls -lh "$OUTPUT_DIR/rock-os.cpio.gz"
else
    echo -e "${RED}✗ Image creation failed${NC}"
    exit 1
fi

# Step 8: Verify with rock-verify
echo ""
echo -e "${BLUE}Step 8: Verifying integration${NC}"
rock-verify integration "$OUTPUT_DIR/rock-os.cpio.gz"
VERIFY_RESULT=$?

if [ $VERIFY_RESULT -eq 0 ]; then
    echo -e "${GREEN}✓ Integration verification PASSED${NC}"
else
    echo -e "${RED}✗ Integration verification FAILED${NC}"
fi

# Step 9: Get kernel if needed
echo ""
echo -e "${BLUE}Step 9: Getting kernel${NC}"
if [ ! -f "${HOME}/.rock/kernels/vmlinuz" ]; then
    echo "Kernel not cached, fetching..."
    rock-kernel fetch alpine:5.10.180
    rock-kernel extract "${HOME}/.rock/kernels/alpine-5.10.180.apk"
fi
cp "${HOME}/.rock/kernels/vmlinuz" "$OUTPUT_DIR/vmlinuz"
echo "✓ Kernel ready"

# Summary
echo ""
echo -e "${GREEN}=====================================${NC}"
echo -e "${GREEN}         TEST COMPLETE!              ${NC}"
echo -e "${GREEN}=====================================${NC}"
echo ""
echo "Output files:"
ls -lh "$OUTPUT_DIR"/*.gz "$OUTPUT_DIR"/vmlinuz
echo ""
echo "Test boot with:"
echo -e "${YELLOW}./scripts/test-boot.sh $OUTPUT_DIR/vmlinuz $OUTPUT_DIR/rock-os.cpio.gz${NC}"
echo ""
echo "Or manually:"
echo -e "${YELLOW}qemu-system-x86_64 -m 512 -kernel $OUTPUT_DIR/vmlinuz -initrd $OUTPUT_DIR/rock-os.cpio.gz -append \"init=/sbin/init console=ttyS0 debug\" -nographic -serial mon:stdio${NC}"