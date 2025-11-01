// rock-image - CRITICAL Image Creation Tool for ROCK-OS
//
// This tool creates initramfs images with EXACT paths required by rock-init.
// Getting the paths wrong means ROCK-OS will not boot.
//
// Usage:
//   rock-image create <rootfs-dir> -o <output.cpio.gz>
//   rock-image validate <image.cpio.gz>
//   rock-image structure                  - Show required structure
//
// Build:
//   go build -o rock-image cmd/rock-image/main.go
//
// CRITICAL: This tool MUST use the paths from pkg/integration

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rock-os/tools/pkg/integration"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// CreateInitramfs creates an initramfs with correct structure
func CreateInitramfs(rootfsPath string, outputPath string) error {
	fmt.Println("Creating initramfs with rock-init integration...")
	fmt.Printf("  Source: %s\n", rootfsPath)
	fmt.Printf("  Output: %s\n", outputPath)
	fmt.Println()

	// First, verify the rootfs has required files
	fmt.Println("Step 1: Verifying rootfs structure...")
	result, err := integration.VerifyRootfs(rootfsPath)
	if err != nil {
		return fmt.Errorf("failed to verify rootfs: %w", err)
	}

	if !result.Success {
		fmt.Println("❌ Rootfs verification failed!")
		integration.PrintVerificationResult(result)
		return fmt.Errorf("rootfs does not meet integration requirements")
	}
	fmt.Println("✅ Rootfs structure verified")

	// Create the output file
	fmt.Println("\nStep 2: Creating archive...")
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Walk the rootfs and add files
	err = filepath.Walk(rootfsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(rootfsPath, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Set the name to the relative path
		header.Name = relPath

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = linkTarget
		}

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file contents (if regular file)
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	fmt.Printf("✅ Archive created: %s\n", outputPath)

	// Verify the created image
	fmt.Println("\nStep 3: Verifying created image...")
	// Close writers first to flush
	tarWriter.Close()
	gzWriter.Close()
	outFile.Close()

	verifyResult, err := integration.VerifyImage(outputPath)
	if err != nil {
		return fmt.Errorf("failed to verify created image: %w", err)
	}

	if !verifyResult.Success {
		fmt.Println("❌ Created image failed verification!")
		integration.PrintVerificationResult(verifyResult)
		return fmt.Errorf("created image does not meet integration requirements")
	}

	fmt.Println("✅ Image verification passed")
	fmt.Printf("\n✅ Successfully created initramfs: %s\n", outputPath)
	return nil
}

// CreateDeviceNode creates a device node (placeholder on macOS)
func CreateDeviceNode(rootfs string, node integration.DeviceNode) error {
	path := filepath.Join(rootfs, node.Path)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// On macOS, we can't create real device nodes, so create placeholders
	// On Linux, this would use mknod system call
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	file.Close()

	if err := os.Chmod(path, os.FileMode(node.Mode)); err != nil {
		return err
	}

	fmt.Printf("  ⚠️  Created placeholder for %s (real device nodes created on Linux)\n", node.Path)
	return nil
}

// ShowRequiredStructure displays the required initramfs structure
func ShowRequiredStructure() {
	fmt.Println("REQUIRED INITRAMFS STRUCTURE FOR ROCK-INIT")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("CRITICAL: These paths are hardcoded in rock-init!")
	fmt.Println()

	contract := integration.GetContract()

	fmt.Println("Required Files:")
	fmt.Println("---------------")
	for _, binary := range contract.Binaries {
		fmt.Printf("  %s\n", binary.Destination)
		if binary.Source != filepath.Base(binary.Destination) {
			fmt.Printf("    (renamed from %s)\n", binary.Source)
		}
	}
	fmt.Println()

	fmt.Println("Required Symlinks to Busybox:")
	fmt.Println("-----------------------------")
	for _, symlink := range integration.BusyboxSymlinks {
		fmt.Printf("  /bin/%s -> busybox\n", symlink)
	}
	fmt.Println()

	fmt.Println("Required Directories:")
	fmt.Println("--------------------")
	for _, dir := range contract.Directories {
		fmt.Printf("  %s/\n", dir)
	}
	fmt.Println()

	fmt.Println("Required Device Nodes:")
	fmt.Println("---------------------")
	for _, node := range contract.DeviceNodes {
		fmt.Printf("  %s (major:%d minor:%d mode:%o)\n",
			node.Path, node.Major, node.Minor, node.Mode)
	}
	fmt.Println()

	fmt.Println("Example rootfs preparation:")
	fmt.Println("---------------------------")
	fmt.Println("  # Create directories")
	fmt.Println("  mkdir -p rootfs/{sbin,bin,usr/bin,etc/rock,config,proc,sys,dev,tmp,run,var/log}")
	fmt.Println()
	fmt.Println("  # Copy binaries (MUST rename rock-init!)")
	fmt.Println("  cp rock-init rootfs/sbin/init")
	fmt.Println("  cp rock-manager rootfs/usr/bin/")
	fmt.Println("  cp volcano-agent rootfs/usr/bin/")
	fmt.Println("  cp busybox rootfs/bin/")
	fmt.Println()
	fmt.Println("  # Create busybox symlinks")
	fmt.Println("  cd rootfs/bin && for cmd in sh ls cat echo mount umount; do ln -s busybox $cmd; done")
	fmt.Println()
	fmt.Println("  # Create image")
	fmt.Println("  rock-image create rootfs -o initrd.cpio.gz")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("rock-image - CRITICAL Image Creation Tool for ROCK-OS")
		fmt.Println()
		fmt.Println("This tool creates initramfs images with the EXACT structure")
		fmt.Println("required by rock-init. Getting paths wrong = no boot!")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  rock-image create <rootfs> -o <output>  Create initramfs")
		fmt.Println("  rock-image validate <image>             Validate an image")
		fmt.Println("  rock-image structure                    Show required structure")
		fmt.Println("  rock-image version                      Show version")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  rock-image create ./rootfs -o initrd.cpio.gz")
		fmt.Println("  rock-image validate initrd.cpio.gz")
		fmt.Println()
		fmt.Println("CRITICAL INTEGRATION RULES:")
		fmt.Println("  1. rock-init MUST be renamed to /sbin/init")
		fmt.Println("  2. rock-manager MUST be at /usr/bin/rock-manager")
		fmt.Println("  3. volcano-agent MUST be at /usr/bin/volcano-agent")
		fmt.Println("  4. Shell MUST be at /bin/sh (symlink to busybox)")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version":
		fmt.Printf("rock-image version %s (built %s, commit %s)\n", Version, BuildTime, GitCommit)
		return

	case "structure":
		ShowRequiredStructure()
		return

	case "validate":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: missing image path")
			fmt.Fprintln(os.Stderr, "Usage: rock-image validate <image.cpio.gz>")
			os.Exit(1)
		}
		result, err := integration.VerifyImage(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		integration.PrintVerificationResult(result)
		if !result.Success {
			os.Exit(1)
		}
		return

	case "create":
		if len(os.Args) < 5 || os.Args[3] != "-o" {
			fmt.Fprintln(os.Stderr, "Error: invalid arguments")
			fmt.Fprintln(os.Stderr, "Usage: rock-image create <rootfs> -o <output.cpio.gz>")
			os.Exit(1)
		}
		rootfs := os.Args[2]
		output := os.Args[4]

		if err := CreateInitramfs(rootfs, output); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}
		return

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		os.Exit(1)
	}
}