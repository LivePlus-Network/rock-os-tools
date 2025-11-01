# Running the Complete ROCK-OS Build Pipeline

## Quick Start (3 Options)

### Option 1: Automated Pipeline (Recommended)
```bash
# Run complete pipeline with rock-compose
rock-compose run pipelines/build-rock-os.yaml

# The pipeline will:
# - Build all components
# - Create initramfs with correct paths
# - Verify integration
# - Prepare for QEMU testing
```

### Option 2: Manual Step-by-Step
```bash
# Run the manual build script
chmod +x scripts/build-rock-os-manual.sh
./scripts/build-rock-os-manual.sh debug    # or 'production'

# This shows each command as it runs
# Educational - see what each tool does
```

### Option 3: Individual Commands
```bash
# Run each step yourself for maximum control
mkdir -p output

# 1. Build components
rock-build all --mode=debug --output=/tmp/rock-build

# 2. Prepare rootfs
rm -rf /tmp/rock-rootfs
mkdir -p /tmp/rock-rootfs/{bin,sbin,usr/bin,etc/rock,config,proc,sys,dev,tmp,run,var/log,lib}

# 3. Install binaries (CRITICAL PATHS!)
cp /tmp/rock-build/rock-init /tmp/rock-rootfs/sbin/init
cp /tmp/rock-build/rock-manager /tmp/rock-rootfs/usr/bin/rock-manager
cp /tmp/rock-build/volcano-agent /tmp/rock-rootfs/usr/bin/volcano-agent
chmod 755 /tmp/rock-rootfs/sbin/init
chmod 755 /tmp/rock-rootfs/usr/bin/rock-manager
chmod 755 /tmp/rock-rootfs/usr/bin/volcano-agent

# 4. Install BusyBox
curl -L -o /tmp/busybox https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
chmod 755 /tmp/busybox
cp /tmp/busybox /tmp/rock-rootfs/bin/busybox
cd /tmp/rock-rootfs/bin && ln -s busybox sh && ln -s busybox ls && cd -

# 5. Copy dependencies
rock-deps copy /tmp/rock-rootfs/sbin/init /tmp/rock-rootfs/lib

# 6. Generate config
rock-config generate node \
  --output=/tmp/rock-rootfs/etc/rock/config.yaml \
  --node-id=test-node \
  --mac=a4:58:0f:00:00:01

# 7. Generate keys (optional)
rock-security keygen --output=/tmp/rock-rootfs/config/CONFIG_KEY

# 8. Create device nodes
cd /tmp/rock-rootfs/dev
sudo mknod -m 666 null c 1 3
sudo mknod -m 666 zero c 1 5
sudo mknod -m 666 random c 1 8
sudo mknod -m 666 urandom c 1 9
cd -

# 9. Build image
rock-image cpio create /tmp/rock-rootfs --output=output/rock-os.cpio.gz

# 10. Verify
rock-verify integration output/rock-os.cpio.gz

# 11. Get kernel
rock-kernel fetch alpine:5.10.180
rock-kernel extract ~/.rock/kernels/alpine-5.10.180.apk
cp ~/.rock/kernels/vmlinuz output/

# 12. Test in QEMU
qemu-system-x86_64 \
  -m 512 \
  -kernel output/vmlinuz \
  -initrd output/rock-os.cpio.gz \
  -append "init=/sbin/init console=ttyS0 debug" \
  -nographic -serial mon:stdio
```

## Testing the Built Image

### Quick Test
```bash
# Use the test script
chmod +x scripts/test-boot.sh
./scripts/test-boot.sh output/vmlinuz output/rock-os.cpio.gz
```

### Manual QEMU Test
```bash
# Basic boot test
qemu-system-x86_64 \
  -m 512 \
  -kernel output/vmlinuz \
  -initrd output/rock-os.cpio.gz \
  -append "init=/sbin/init console=ttyS0 debug" \
  -nographic -serial mon:stdio

# With networking (if socket_vmnet is running)
qemu-system-x86_64 \
  -m 512 \
  -kernel output/vmlinuz \
  -initrd output/rock-os.cpio.gz \
  -append "init=/sbin/init console=ttyS0 debug" \
  -netdev socket,id=net0,fd=3 3<>/var/run/socket_vmnet \
  -device virtio-net-pci,netdev=net0,mac=a4:58:0f:00:00:01 \
  -nographic -serial mon:stdio
```

**Exit QEMU:** Press `Ctrl-A` then `X`

## Verification Checklist

Before testing in QEMU, ensure:
- ✅ `rock-verify integration` passes
- ✅ `/sbin/init` exists (rock-init renamed)
- ✅ `/usr/bin/rock-manager` exists
- ✅ `/usr/bin/volcano-agent` exists
- ✅ `/bin/busybox` and `/bin/sh` symlink exist
- ✅ Image size is reasonable (10-50MB)

## Troubleshooting

### Build Failures
```bash
# Check individual tool output
rock-build all --verbose

# Verify Rust toolchain
rustc --version
cargo --version

# Check target
rustup target list | grep musl
```

### Integration Failures
```bash
# Extract and inspect image
rock-image cpio extract output/rock-os.cpio.gz
ls -la /tmp/extracted/sbin/init
ls -la /tmp/extracted/usr/bin/

# Use ROCK-MASTER verification
../ROCK-MASTER/verify-rock-init-integration.sh output/rock-os.cpio.gz
```

### Boot Failures
```bash
# Enable debug output
rock-kernel cmdline debug  # Should show: init=/sbin/init console=ttyS0 debug

# Check kernel
file output/vmlinuz  # Should be: Linux kernel x86 boot executable

# Try with more memory
qemu-system-x86_64 -m 1024 ...  # Use 1GB instead of 512MB
```

## Success Indicators

When everything works, you'll see:
1. **Build:** "✓ Build complete! Image at: output/rock-os.cpio.gz"
2. **Verify:** "✓ Image is fully compatible with rock-init!"
3. **Boot:** Rock-init starts, mounts filesystems, starts services

## Integration with ROCK-MASTER

Once validated, use the image with ROCK-MASTER:
```bash
# Copy to ROCK-MASTER images directory
cp output/rock-os.cpio.gz /Volumes/4TB/ROCK-MASTER/images/
cp output/vmlinuz /Volumes/4TB/ROCK-MASTER/images/

# Test with ROCK-MASTER scripts
cd /Volumes/4TB/ROCK-MASTER
./scripts/09_test_boot.sh
```

## Next Steps

After successful build and boot:
1. **Customize configuration** - Modify pipeline YAML
2. **Add components** - Include additional binaries
3. **Production build** - Use `--mode=production`
4. **Create ISO** - Add rock-image iso support
5. **CI/CD integration** - Automate builds

---

**Remember:** The critical requirement is that rock-init finds binaries at exact paths:
- `/sbin/init` (rock-init itself, renamed)
- `/usr/bin/rock-manager`
- `/usr/bin/volcano-agent`

Get these wrong and nothing boots!