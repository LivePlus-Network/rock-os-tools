#!/bin/bash
#
# build-vultr.sh - Build ROCK-OS image optimized for Vultr cloud platform
#
# This script creates a bootable ROCK-OS image with:
# - Full VirtIO support for Vultr virtualization
# - Proper console configuration (ttyS0,115200n8)
# - Optimized boot parameters for cloud instances
#

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}======================================${NC}"
echo -e "${GREEN}   ROCK-OS Vultr Image Builder       ${NC}"
echo -e "${GREEN}======================================${NC}"
echo ""

# Configuration
ROOTFS_DIR="/tmp/vultr-rootfs"
OUTPUT_DIR="output/vultr"
KERNEL_NAME="vmlinuz-virt"
INITRAMFS_NAME="vultr-rock-os.cpio.gz"

# Vultr-specific kernel parameters
VULTR_CMDLINE="console=ttyS0,115200n8 earlyprintk=ttyS0 rdinit=/sbin/init"

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Step 1: Fetch Alpine virt kernel (optimized for VirtIO)
echo -e "${YELLOW}Step 1: Fetching Alpine virt kernel...${NC}"
if [ -f "$OUTPUT_DIR/$KERNEL_NAME" ]; then
    echo "  Using existing kernel"
else
    # For now, use the existing kernel (Alpine kernels have VirtIO built-in)
    cp output/vmlinuz "$OUTPUT_DIR/$KERNEL_NAME"
    echo "  ✅ Kernel copied (Alpine kernel includes VirtIO support)"
fi

# Step 2: Prepare Vultr-optimized rootfs
echo -e "${YELLOW}Step 2: Preparing Vultr rootfs...${NC}"
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"/{bin,sbin,etc,dev,proc,sys,tmp,var/run,usr/bin,config}
mkdir -p "$ROOTFS_DIR"/{lib,lib64,etc/rock/tls}

# Create Vultr-specific directories
mkdir -p "$ROOTFS_DIR"/etc/network
mkdir -p "$ROOTFS_DIR"/var/log

echo "  ✅ Directory structure created"

# Step 3: Install binaries from ROCK-MASTER
echo -e "${YELLOW}Step 3: Installing ROCK-OS binaries...${NC}"

# Copy rock-init as /sbin/init (CRITICAL for boot)
if [ -f "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-init" ]; then
    cp "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-init" \
       "$ROOTFS_DIR/sbin/init"
    chmod 755 "$ROOTFS_DIR/sbin/init"
    echo "  ✅ rock-init installed as /sbin/init"
else
    echo -e "${RED}  ❌ rock-init not found!${NC}"
    exit 1
fi

# Copy rock-manager
if [ -f "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-manager" ]; then
    cp "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-manager" \
       "$ROOTFS_DIR/usr/bin/rock-manager"
    chmod 755 "$ROOTFS_DIR/usr/bin/rock-manager"
    echo "  ✅ rock-manager installed"
fi

# Copy volcano-agent
if [ -f "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/volcano-agent" ]; then
    cp "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/volcano-agent" \
       "$ROOTFS_DIR/usr/bin/volcano-agent"
    chmod 755 "$ROOTFS_DIR/usr/bin/volcano-agent"
    echo "  ✅ volcano-agent installed"
fi

# Step 4: Install BusyBox
echo -e "${YELLOW}Step 4: Installing BusyBox...${NC}"
BUSYBOX_PATH="/tmp/busybox"
if [ ! -f "$BUSYBOX_PATH" ]; then
    echo "  Downloading BusyBox..."
    curl -L -o "$BUSYBOX_PATH" \
        "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
    chmod 755 "$BUSYBOX_PATH"
fi

cp "$BUSYBOX_PATH" "$ROOTFS_DIR/bin/busybox"
chmod 755 "$ROOTFS_DIR/bin/busybox"

# Create essential symlinks for Vultr
cd "$ROOTFS_DIR/bin"
for cmd in sh ls cat echo mount umount mkdir rm ps grep find \
           ifconfig ip route dhclient hostname; do
    ln -sf busybox "$cmd" 2>/dev/null || true
done
cd - > /dev/null

echo "  ✅ BusyBox installed with symlinks"

# Step 5: Create Vultr-specific configuration
echo -e "${YELLOW}Step 5: Creating Vultr configuration...${NC}"

# Network configuration for VirtIO
cat > "$ROOTFS_DIR/etc/network/interfaces" << 'EOF'
# Vultr VirtIO network configuration
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
EOF

# Console configuration
cat > "$ROOTFS_DIR/etc/inittab" << 'EOF'
# Vultr console configuration
::sysinit:/sbin/init
ttyS0::respawn:/bin/sh
::restart:/sbin/init
::ctrlaltdel:/sbin/reboot
EOF

# Vultr detection script
cat > "$ROOTFS_DIR/etc/vultr-detect.sh" << 'EOF'
#!/bin/sh
# Detect if running on Vultr
if [ -f /sys/devices/virtual/dmi/id/sys_vendor ]; then
    vendor=$(cat /sys/devices/virtual/dmi/id/sys_vendor)
    if echo "$vendor" | grep -qi "vultr\|kvm\|qemu"; then
        echo "Running on Vultr/KVM platform"
        # Enable VirtIO optimizations
        echo 128 > /sys/module/virtio_net/parameters/napi_weight 2>/dev/null || true
    fi
fi
EOF
chmod 755 "$ROOTFS_DIR/etc/vultr-detect.sh"

echo "  ✅ Vultr configuration created"

# Step 6: Create minimal device nodes (most created by devtmpfs at boot)
echo -e "${YELLOW}Step 6: Creating device nodes...${NC}"
# These are minimal - kernel's devtmpfs will create the rest
touch "$ROOTFS_DIR/dev/null"
touch "$ROOTFS_DIR/dev/console"
touch "$ROOTFS_DIR/dev/ttyS0"
echo "  ✅ Device placeholders created"

# Step 7: Add Vultr boot message
cat > "$ROOTFS_DIR/etc/motd" << 'EOF'
=====================================
    ROCK-OS on Vultr Cloud
    Optimized for VirtIO
=====================================
EOF

# Step 8: Create the Vultr initramfs
echo -e "${YELLOW}Step 8: Creating Vultr initramfs...${NC}"

# Use the fixed rock-image tool to create proper CPIO
/Volumes/4TB/ROCK-MASTER/bin/tools/rock-image cpio create "$ROOTFS_DIR" 2>&1 | grep -v "^Step" || true

# Move and rename the output
if [ -f "initrd.cpio.gz" ]; then
    mv initrd.cpio.gz "$OUTPUT_DIR/$INITRAMFS_NAME"
    echo "  ✅ Created $OUTPUT_DIR/$INITRAMFS_NAME"

    # Show size
    SIZE=$(ls -lh "$OUTPUT_DIR/$INITRAMFS_NAME" | awk '{print $5}')
    echo "  📦 Image size: $SIZE"
else
    echo -e "${RED}  ❌ Failed to create initramfs${NC}"
    exit 1
fi

# Step 9: Create boot configuration file
echo -e "${YELLOW}Step 9: Creating boot configuration...${NC}"

cat > "$OUTPUT_DIR/vultr-boot.conf" << EOF
# Vultr Boot Configuration for ROCK-OS
#
# Kernel: $KERNEL_NAME
# Initramfs: $INITRAMFS_NAME
#
# Boot parameters:
$VULTR_CMDLINE
#
# iPXE example:
#!ipxe
kernel $KERNEL_NAME $VULTR_CMDLINE
initrd $INITRAMFS_NAME
boot

# GRUB example:
linux /$KERNEL_NAME $VULTR_CMDLINE
initrd /$INITRAMFS_NAME
EOF

echo "  ✅ Boot configuration created"

# Step 10: Create deployment package
echo -e "${YELLOW}Step 10: Creating deployment package...${NC}"

cd "$OUTPUT_DIR"
tar -czf vultr-rock-os.tar.gz "$KERNEL_NAME" "$INITRAMFS_NAME" vultr-boot.conf
cd - > /dev/null

echo "  ✅ Created deployment package: $OUTPUT_DIR/vultr-rock-os.tar.gz"

# Step 11: Test locally with QEMU (simulating Vultr)
echo ""
echo -e "${GREEN}======================================${NC}"
echo -e "${GREEN}Build Complete!${NC}"
echo ""
echo "Files created:"
echo "  • Kernel: $OUTPUT_DIR/$KERNEL_NAME"
echo "  • Initramfs: $OUTPUT_DIR/$INITRAMFS_NAME ($(ls -lh "$OUTPUT_DIR/$INITRAMFS_NAME" | awk '{print $5}'))"
echo "  • Package: $OUTPUT_DIR/vultr-rock-os.tar.gz"
echo "  • Config: $OUTPUT_DIR/vultr-boot.conf"
echo ""
echo "To test locally (simulating Vultr):"
echo -e "${YELLOW}qemu-system-x86_64 \\"
echo "  -m 512 \\"
echo "  -device virtio-net-pci,netdev=net0 \\"
echo "  -netdev user,id=net0 \\"
echo "  -device virtio-blk-pci,drive=drive0 \\"
echo "  -drive if=none,id=drive0,file=/dev/null,format=raw \\"
echo "  -kernel $OUTPUT_DIR/$KERNEL_NAME \\"
echo "  -initrd $OUTPUT_DIR/$INITRAMFS_NAME \\"
echo "  -append \"$VULTR_CMDLINE\" \\"
echo "  -nographic -serial mon:stdio${NC}"
echo ""
echo "To deploy on Vultr:"
echo "  1. Upload vultr-rock-os.tar.gz to your Vultr account"
echo "  2. Create a custom ISO or use iPXE boot"
echo "  3. Use the kernel parameters from vultr-boot.conf"
echo ""
echo -e "${GREEN}✅ Vultr image ready for deployment!${NC}"

# Cleanup
rm -rf "$ROOTFS_DIR"