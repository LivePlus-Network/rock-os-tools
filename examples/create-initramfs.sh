#!/bin/bash
#
# Example: Creating a ROCK-OS initramfs with rock-image
# This demonstrates the critical integration requirements
#

set -e

echo "ROCK-OS Initramfs Creation Example"
echo "==================================="
echo

# Check for required binaries
echo "Checking for required binaries..."
REQUIRED_BINARIES=(
    "rock-init"
    "rock-manager"
    "volcano-agent"
    "busybox"
)

MISSING=()
for binary in "${REQUIRED_BINARIES[@]}"; do
    # Check in common locations
    if [ -f "../rock-os/target/debug/$binary" ]; then
        echo "  ✓ Found $binary"
    elif [ -f "../rock-os/target/release/$binary" ]; then
        echo "  ✓ Found $binary"
    elif [ -f "/tmp/$binary" ]; then
        echo "  ✓ Found $binary"
    else
        echo "  ✗ Missing $binary"
        MISSING+=("$binary")
    fi
done

if [ ${#MISSING[@]} -gt 0 ]; then
    echo
    echo "ERROR: Missing binaries: ${MISSING[*]}"
    echo
    echo "To test this script, you need to build or obtain:"
    echo "  - rock-init (from rock-os/init)"
    echo "  - rock-manager (from rock-os/manager)"
    echo "  - volcano-agent (from rock-os/agent)"
    echo "  - busybox (download from https://busybox.net/downloads/binaries/)"
    echo
    echo "Place them in one of these locations:"
    echo "  - ../rock-os/target/debug/"
    echo "  - ../rock-os/target/release/"
    echo "  - /tmp/"
    exit 1
fi

echo
echo "Creating rootfs directory structure..."

# Clean up any existing rootfs
rm -rf rootfs
mkdir -p rootfs

# Create required directories
mkdir -p rootfs/{bin,sbin,etc,dev,proc,sys,tmp,var/run,usr/bin}
mkdir -p rootfs/{lib,lib64,config,etc/rock/tls}

echo "  ✓ Created directory structure"

# Copy binaries to correct locations
echo
echo "Copying binaries (with CRITICAL renaming)..."

# Find and copy rock-init (MUST BE RENAMED!)
if [ -f "../rock-os/target/debug/rock-init" ]; then
    cp ../rock-os/target/debug/rock-init rootfs/sbin/init
elif [ -f "../rock-os/target/release/rock-init" ]; then
    cp ../rock-os/target/release/rock-init rootfs/sbin/init
elif [ -f "/tmp/rock-init" ]; then
    cp /tmp/rock-init rootfs/sbin/init
fi
echo "  ✓ Copied rock-init → /sbin/init (RENAMED!)"

# Copy rock-manager
if [ -f "../rock-os/target/debug/rock-manager" ]; then
    cp ../rock-os/target/debug/rock-manager rootfs/usr/bin/
elif [ -f "../rock-os/target/release/rock-manager" ]; then
    cp ../rock-os/target/release/rock-manager rootfs/usr/bin/
elif [ -f "/tmp/rock-manager" ]; then
    cp /tmp/rock-manager rootfs/usr/bin/
fi
echo "  ✓ Copied rock-manager → /usr/bin/rock-manager"

# Copy volcano-agent
if [ -f "../rock-os/target/debug/volcano-agent" ]; then
    cp ../rock-os/target/debug/volcano-agent rootfs/usr/bin/
elif [ -f "../rock-os/target/release/volcano-agent" ]; then
    cp ../rock-os/target/release/volcano-agent rootfs/usr/bin/
elif [ -f "/tmp/volcano-agent" ]; then
    cp /tmp/volcano-agent rootfs/usr/bin/
fi
echo "  ✓ Copied volcano-agent → /usr/bin/volcano-agent"

# Copy busybox
if [ -f "../rock-os/build/busybox" ]; then
    cp ../rock-os/build/busybox rootfs/bin/
elif [ -f "/tmp/busybox" ]; then
    cp /tmp/busybox rootfs/bin/
fi
echo "  ✓ Copied busybox → /bin/busybox"

# Create busybox symlinks
echo
echo "Creating busybox symlinks..."
cd rootfs/bin

# Essential symlinks
SYMLINKS=(
    sh ls cat ps echo sleep
    mount umount mkdir rm cp mv
    grep awk sed find ifconfig route
    top tail head dmesg kill free df
    ping netstat wget nc ip vi
)

for cmd in "${SYMLINKS[@]}"; do
    ln -sf busybox "$cmd"
done

echo "  ✓ Created ${#SYMLINKS[@]} busybox symlinks"

cd ../..

# Create device nodes (if running as root)
if [ "$EUID" -eq 0 ]; then
    echo
    echo "Creating device nodes (running as root)..."
    mknod rootfs/dev/null c 1 3
    mknod rootfs/dev/zero c 1 5
    mknod rootfs/dev/random c 1 8
    mknod rootfs/dev/urandom c 1 9
    mknod rootfs/dev/tty c 5 0
    mknod rootfs/dev/console c 5 1
    mknod rootfs/dev/ptmx c 5 2
    echo "  ✓ Created device nodes"
else
    echo
    echo "⚠️  Skipping device nodes (not running as root)"
    echo "   Device nodes would be created with:"
    echo "   sudo mknod rootfs/dev/null c 1 3"
    echo "   sudo mknod rootfs/dev/console c 5 1"
    echo "   etc..."
fi

# Create the initramfs
echo
echo "Creating CPIO initramfs..."
echo "=========================="
echo

# Use rock-image to create the CPIO archive
if [ -f "./bin/darwin/rock-image" ]; then
    ROCK_IMAGE="./bin/darwin/rock-image"
elif [ -f "./rock-image" ]; then
    ROCK_IMAGE="./rock-image"
else
    echo "ERROR: rock-image not found!"
    echo "Build it with: make rock-image"
    exit 1
fi

$ROCK_IMAGE cpio create rootfs

# Verify the image
echo
echo "Verifying the created image..."
echo "=============================="
echo

$ROCK_IMAGE cpio verify initrd.cpio.gz

echo
echo "SUCCESS! The initramfs is ready: initrd.cpio.gz"
echo
echo "You can now boot with:"
echo "  qemu-system-x86_64 -kernel vmlinuz -initrd initrd.cpio.gz"
echo
echo "Or extract and inspect with:"
echo "  $ROCK_IMAGE cpio extract initrd.cpio.gz"