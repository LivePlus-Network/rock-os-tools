# Boot Fix Required

## Problem Identified
The kernel panics with "VFS: Unable to mount root fs" even though our initramfs contains all the right files.

## Root Cause
The cpio archive has **relative paths** with leading "./"
```
./sbin/init
./usr/bin/rock-manager
```

But the kernel expects **absolute paths** without the dot:
```
sbin/init
usr/bin/rock-manager
```

## Solution
The rock-image tool needs to create the cpio archive without the leading "./" prefix.

## Current cpio command (in rock-image):
```bash
cd /tmp/rock-rootfs && find . | cpio -o -H newc | gzip > output.cpio.gz
```

## Fixed command should be:
```bash
cd /tmp/rock-rootfs && find . -mindepth 1 | sed 's|^\./||' | cpio -o -H newc | gzip > output.cpio.gz
```

Or use the cpio --no-absolute-filenames option correctly.

## Quick Workaround
Create the image manually with correct format:
```bash
cd /tmp/rock-rootfs
find . -mindepth 1 | sed 's|^\./||' | cpio -o -H newc | gzip > /tmp/fixed.cpio.gz
```

## Test Status
- ✅ Kernel boots
- ✅ Initramfs loads (37MB)
- ❌ Can't mount root (path format issue)
- ⏳ rock-init hasn't run yet

Once this is fixed, rock-init should start as PID 1!