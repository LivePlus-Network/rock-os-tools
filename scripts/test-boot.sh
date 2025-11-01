#!/bin/bash
#
# test-boot.sh - Test ROCK-OS image in QEMU
# Usage: ./test-boot.sh <vmlinuz> <initramfs.cpio.gz>
#

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Check arguments
if [ $# -ne 2 ]; then
    echo "Usage: $0 <vmlinuz> <initramfs.cpio.gz>"
    echo "Example: $0 output/vmlinuz output/rock-os.cpio.gz"
    exit 1
fi

KERNEL="$1"
INITRAMFS="$2"

# Verify files exist
if [ ! -f "$KERNEL" ]; then
    echo -e "${RED}Error: Kernel not found: $KERNEL${NC}"
    exit 1
fi

if [ ! -f "$INITRAMFS" ]; then
    echo -e "${RED}Error: Initramfs not found: $INITRAMFS${NC}"
    exit 1
fi

echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}    ROCK-OS QEMU Boot Test       ${NC}"
echo -e "${GREEN}==================================${NC}"
echo ""
echo "Kernel:    $(basename $KERNEL) ($(ls -lh $KERNEL | awk '{print $5}'))"
echo "Initramfs: $(basename $INITRAMFS) ($(ls -lh $INITRAMFS | awk '{print $5}'))"
echo ""

# Get kernel cmdline from rock-kernel tool (rdinit for initramfs)
CMDLINE=$(rock-kernel cmdline debug 2>/dev/null || echo "rdinit=/sbin/init console=ttyS0 debug")
echo "Cmdline: $CMDLINE"
echo ""

# Check if socket_vmnet is available for networking
NETWORK_ARGS=""
if pgrep -f socket_vmnet > /dev/null 2>&1; then
    echo -e "${GREEN}✓ socket_vmnet detected - enabling bridge networking${NC}"
    SOCKET_PATH="/var/run/socket_vmnet"

    # Find the bridge interface
    BRIDGE_IF=$(find /var/run -name "bridge*" 2>/dev/null | head -1)

    if [ -n "$BRIDGE_IF" ]; then
        NETWORK_ARGS="-netdev socket,id=net0,fd=3 3<>/var/run/socket_vmnet -device virtio-net-pci,netdev=net0,mac=a4:58:0f:00:00:01"
        echo "  Using bridge: $BRIDGE_IF"
        echo "  MAC address: a4:58:0f:00:00:01"
    else
        echo -e "${YELLOW}⚠ socket_vmnet running but bridge not found${NC}"
        NETWORK_ARGS="-netdev user,id=net0 -device virtio-net-pci,netdev=net0,mac=a4:58:0f:00:00:01"
    fi
else
    echo -e "${YELLOW}⚠ socket_vmnet not running - using user networking${NC}"
    echo "  For bridge networking, start socket_vmnet first"
    NETWORK_ARGS="-netdev user,id=net0 -device virtio-net-pci,netdev=net0,mac=a4:58:0f:00:00:01"
fi

echo ""
echo -e "${YELLOW}Starting QEMU...${NC}"
echo "Press Ctrl-A X to exit"
echo ""
echo "=================================="
echo ""

# Run QEMU
# -m 512: 512MB RAM
# -cpu host: Use host CPU features
# -enable-kvm: Enable KVM acceleration (Linux only)
# -nographic: No GUI, use serial console
# -serial mon:stdio: Redirect serial to terminal
# -append: Kernel command line

if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS - no KVM
    qemu-system-x86_64 \
        -m 512 \
        -kernel "$KERNEL" \
        -initrd "$INITRAMFS" \
        -append "$CMDLINE" \
        $NETWORK_ARGS \
        -nographic \
        -serial mon:stdio
else
    # Linux - use KVM if available
    KVM_ARGS=""
    if [ -w /dev/kvm ]; then
        KVM_ARGS="-enable-kvm -cpu host"
        echo "KVM acceleration enabled"
    fi

    qemu-system-x86_64 \
        -m 512 \
        $KVM_ARGS \
        -kernel "$KERNEL" \
        -initrd "$INITRAMFS" \
        -append "$CMDLINE" \
        $NETWORK_ARGS \
        -nographic \
        -serial mon:stdio
fi

echo ""
echo "=================================="
echo -e "${GREEN}QEMU exited${NC}"