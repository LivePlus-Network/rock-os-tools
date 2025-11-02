#!/bin/bash
#
# build-experiment.sh - Build ROCK-OS image for experiment with bridge networking
#
# This creates an image with rock-init's built-in DHCP and networking support
# configured for bridge networking via socket_vmnet
#

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}======================================${NC}"
echo -e "${GREEN}   ROCK-OS Experiment Image Builder  ${NC}"
echo -e "${GREEN}======================================${NC}"
echo ""
echo "This builds an image with rock-init's native networking for:"
echo "  • Bridge networking via socket_vmnet"
echo "  • DHCP client (built into rock-init)"
echo "  • Volcano connection support"
echo ""

# Configuration
ROOTFS_DIR="/tmp/experiment-rootfs"
EXPERIMENT_DIR="/Volumes/4TB/ROCK-MASTER/experiment"
OUTPUT_NAME="rock-volcano-test.cpio.gz"
OUTPUT_DIR="/Volumes/4TB/ROCK-MASTER/rock-os/build"

# Network parameters for rock-init
VOLCANO_SERVER="192.168.1.101:50061"
MAC_PREFIX="a4:58:0f"

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Step 1: Prepare rootfs
echo -e "${YELLOW}Step 1: Preparing experiment rootfs...${NC}"
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"/{bin,sbin,etc,dev,proc,sys,tmp,var/run,usr/bin,config}
mkdir -p "$ROOTFS_DIR"/{lib,lib64,etc/rock/tls,var/log}

echo "  ✅ Directory structure created"

# Step 2: Install ROCK-OS binaries
echo -e "${YELLOW}Step 2: Installing ROCK-OS binaries...${NC}"

# Copy rock-init as /sbin/init (with networking support) - use release build
if [ -f "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/release/rock-init" ]; then
    cp "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/release/rock-init" \
       "$ROOTFS_DIR/sbin/init"
    chmod 755 "$ROOTFS_DIR/sbin/init"
    echo "  ✅ rock-init installed (release build with DHCP/networking)"
elif [ -f "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-init" ]; then
    cp "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/rock-init" \
       "$ROOTFS_DIR/sbin/init"
    chmod 755 "$ROOTFS_DIR/sbin/init"
    echo "  ✅ rock-init installed (debug build with DHCP/networking)"
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

# Copy volcano-agent (will connect over bridge network)
if [ -f "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/volcano-agent" ]; then
    cp "/Volumes/4TB/ROCK-MASTER/rock-os/target/x86_64-unknown-linux-musl/debug/volcano-agent" \
       "$ROOTFS_DIR/usr/bin/volcano-agent"
    chmod 755 "$ROOTFS_DIR/usr/bin/volcano-agent"
    echo "  ✅ volcano-agent installed"
fi

# Step 3: Install minimal BusyBox for debugging
echo -e "${YELLOW}Step 3: Installing BusyBox...${NC}"
BUSYBOX_PATH="/tmp/busybox"
if [ ! -f "$BUSYBOX_PATH" ]; then
    echo "  Downloading BusyBox..."
    curl -L -o "$BUSYBOX_PATH" \
        "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
    chmod 755 "$BUSYBOX_PATH"
fi

cp "$BUSYBOX_PATH" "$ROOTFS_DIR/bin/busybox"
chmod 755 "$ROOTFS_DIR/bin/busybox"

# Create essential symlinks
cd "$ROOTFS_DIR/bin"
for cmd in sh ls cat echo mount umount mkdir rm ps grep find \
           ip route hostname; do
    ln -sf busybox "$cmd" 2>/dev/null || true
done
cd - > /dev/null

echo "  ✅ BusyBox installed with networking tools"

# Step 4: Create network configuration for rock-init
echo -e "${YELLOW}Step 4: Creating network configuration...${NC}"

# Create a configuration that tells rock-init to use DHCP
cat > "$ROOTFS_DIR/etc/network.conf" << EOF
# Network configuration for rock-init
# Bridge networking via socket_vmnet
NETWORK_MODE=dhcp
INTERFACE=eth0
MAC_PREFIX=$MAC_PREFIX
VOLCANO_SERVER=$VOLCANO_SERVER
EOF

# Create rock-specific configuration
mkdir -p "$ROOTFS_DIR/config"
cat > "$ROOTFS_DIR/config/rock.conf" << EOF
# ROCK-OS Configuration for Experiment
network:
  mode: dhcp
  interface: eth0
  mac_prefix: $MAC_PREFIX
  bridge: true

volcano:
  server: $VOLCANO_SERVER
  retry_interval: 5
  max_retries: 10

debug:
  enabled: true
  console: ttyS0
EOF

echo "  ✅ Network configuration created"

# Step 5: Create startup script for rock-init
echo -e "${YELLOW}Step 5: Creating startup configuration...${NC}"

# Create a startup script that rock-init will execute
cat > "$ROOTFS_DIR/etc/rc.local" << 'EOF'
#!/bin/sh
# Startup script executed by rock-init after network is up

echo "Network configuration:"
ip addr show
ip route show

echo "Testing connectivity to Volcano..."
if ping -c 1 192.168.1.101 > /dev/null 2>&1; then
    echo "✓ Network connectivity established"
else
    echo "✗ Cannot reach Volcano server"
fi
EOF
chmod 755 "$ROOTFS_DIR/etc/rc.local"

echo "  ✅ Startup configuration created"

# Step 6: Add socket_vmnet detection
cat > "$ROOTFS_DIR/etc/detect-bridge.sh" << 'EOF'
#!/bin/sh
# Detect if running with socket_vmnet bridge
if ip link show eth0 | grep -q "a4:58:0f"; then
    echo "Bridge networking detected (socket_vmnet)"
    # Rock-init will handle DHCP automatically
fi
EOF
chmod 755 "$ROOTFS_DIR/etc/detect-bridge.sh"

# Step 7: Create minimal device nodes
echo -e "${YELLOW}Step 7: Creating device nodes...${NC}"
# Create actual device nodes (will be overridden by devtmpfs at boot, but needed for init)
mknod "$ROOTFS_DIR/dev/null" c 1 3 2>/dev/null || touch "$ROOTFS_DIR/dev/null"
mknod "$ROOTFS_DIR/dev/console" c 5 1 2>/dev/null || touch "$ROOTFS_DIR/dev/console"
mknod "$ROOTFS_DIR/dev/ttyS0" c 4 64 2>/dev/null || touch "$ROOTFS_DIR/dev/ttyS0"
mknod "$ROOTFS_DIR/dev/zero" c 1 5 2>/dev/null || touch "$ROOTFS_DIR/dev/zero"
mknod "$ROOTFS_DIR/dev/random" c 1 8 2>/dev/null || touch "$ROOTFS_DIR/dev/random"
mknod "$ROOTFS_DIR/dev/urandom" c 1 9 2>/dev/null || touch "$ROOTFS_DIR/dev/urandom"
echo "  ✅ Device nodes created (or placeholders if mknod failed)"

# Step 8: Create the initramfs
echo -e "${YELLOW}Step 8: Creating experiment initramfs...${NC}"

# Use rock-image tool to create proper CPIO
/Volumes/4TB/ROCK-MASTER/bin/tools/rock-image cpio create "$ROOTFS_DIR" 2>&1 | grep -v "^Step" || true

# Move to experiment location
if [ -f "initrd.cpio.gz" ]; then
    mv initrd.cpio.gz "$OUTPUT_DIR/$OUTPUT_NAME"
    echo "  ✅ Created $OUTPUT_DIR/$OUTPUT_NAME"

    SIZE=$(ls -lh "$OUTPUT_DIR/$OUTPUT_NAME" | awk '{print $5}')
    echo "  📦 Image size: $SIZE"
else
    echo -e "${RED}  ❌ Failed to create initramfs${NC}"
    exit 1
fi

# Step 9: Copy kernel to experiment location
echo -e "${YELLOW}Step 9: Setting up experiment kernel...${NC}"

KERNEL_DIR="/Volumes/4TB/ROCK-MASTER/downloads/kernels/alpine-virt-6.6.4-1"
mkdir -p "$KERNEL_DIR"

# Copy kernel from our tools
if [ -f "/Volumes/4TB/rock-os-tools/output/vmlinuz" ]; then
    cp "/Volumes/4TB/rock-os-tools/output/vmlinuz" "$KERNEL_DIR/vmlinuz"
    echo "  ✅ Kernel copied to experiment location"
elif [ ! -f "$KERNEL_DIR/vmlinuz" ]; then
    echo -e "${YELLOW}  ⚠ Kernel not found, experiment script will need it${NC}"
fi

# Step 10: Create test script
echo -e "${YELLOW}Step 10: Creating test script...${NC}"

cat > "$OUTPUT_DIR/test-network.sh" << 'EOF'
#!/bin/bash
# Test the experiment image with bridge networking

# Check if socket_vmnet is running
if ! pgrep -f socket_vmnet > /dev/null; then
    echo "Error: socket_vmnet not running"
    echo "Start with: sudo brew services start socket_vmnet"
    exit 1
fi

echo "Testing ROCK-OS with bridge networking..."
echo "Expected: rock-init will use DHCP to get IP address"
echo ""

cd /Volumes/4TB/ROCK-MASTER
exec ./experiment/test-rock-os.sh
EOF
chmod +x "$OUTPUT_DIR/test-network.sh"

echo "  ✅ Test script created"

# Cleanup
rm -rf "$ROOTFS_DIR"

echo ""
echo -e "${GREEN}======================================${NC}"
echo -e "${GREEN}Build Complete!${NC}"
echo ""
echo "Image created: $OUTPUT_DIR/$OUTPUT_NAME ($(ls -lh "$OUTPUT_DIR/$OUTPUT_NAME" | awk '{print $5}'))"
echo ""
echo "Rock-init networking features:"
echo "  • Built-in DHCP client"
echo "  • Automatic IP configuration"
echo "  • Bridge network support"
echo "  • Volcano connection over bridge"
echo ""
echo "To test with bridge networking:"
echo -e "${YELLOW}cd /Volumes/4TB/ROCK-MASTER${NC}"
echo -e "${YELLOW}./experiment/test-rock-os.sh${NC}"
echo ""
echo "Or use the test script:"
echo -e "${YELLOW}$OUTPUT_DIR/test-network.sh${NC}"
echo ""
echo "Requirements:"
echo "  1. socket_vmnet must be running:"
echo "     ${YELLOW}sudo brew services start socket_vmnet${NC}"
echo "  2. Bridge must be configured"
echo "  3. Volcano server at 192.168.1.101:50061"
echo ""
echo -e "${GREEN}✅ Experiment image ready with rock-init networking!${NC}"