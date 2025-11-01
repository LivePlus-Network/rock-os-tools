// rock-image - CRITICAL Image Creation Tool for ROCK-OS
//
// This tool creates initramfs images with EXACT paths required by rock-init.
// Getting the paths wrong means ROCK-OS will not boot.
//
// Usage:
//   rock-image cpio create <rootfs-dir>    - Create initramfs from directory
//   rock-image cpio extract <image.cpio.gz> - Extract for inspection
//   rock-image cpio verify <image.cpio.gz>  - Verify rock-init integration
//   rock-image structure                    - Show required structure
//
// Build:
//   go build -o rock-image cmd/rock-image/main.go
//
// CRITICAL: This tool MUST use the paths from pkg/integration

package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rock-os/tools/pkg/integration"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// CreateCPIO creates a CPIO archive from a rootfs directory
func CreateCPIO(rootfsPath string) error {
	// First verify the rootfs structure
	fmt.Println("Step 1: Verifying rootfs structure...")
	if err := verifyRootfsStructure(rootfsPath); err != nil {
		return fmt.Errorf("rootfs verification failed: %w", err)
	}
	fmt.Println("✅ Rootfs structure verified")

	// Generate output filename
	outputPath := "initrd.cpio.gz"
	fmt.Printf("\nStep 2: Creating CPIO archive: %s\n", outputPath)

	// Use the system cpio command for compatibility
	// The newc format is required for Linux initramfs
	tempCpio := "initrd.cpio"

	// Build file list
	var files []string
	err := filepath.Walk(rootfsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(rootfsPath, path)
		if relPath != "." {
			files = append(files, relPath)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk rootfs: %w", err)
	}

	// Create CPIO using find and cpio commands
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("cd %s && find . -print | cpio -o -H newc > %s/%s 2>/dev/null",
			rootfsPath, filepath.Dir(outputPath), tempCpio))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create cpio: %v\nOutput: %s", err, output)
	}

	fmt.Printf("  Created CPIO archive (%d files)\n", len(files))

	// Compress with gzip
	fmt.Println("\nStep 3: Compressing with gzip...")
	cpioData, err := ioutil.ReadFile(tempCpio)
	if err != nil {
		return fmt.Errorf("failed to read cpio: %w", err)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	if _, err := gzWriter.Write(cpioData); err != nil {
		return fmt.Errorf("failed to compress: %w", err)
	}

	// Clean up temp file
	os.Remove(tempCpio)

	// Get file size
	gzWriter.Close()
	outFile.Close()
	stat, _ := os.Stat(outputPath)
	fmt.Printf("  Compressed size: %.2f MB\n", float64(stat.Size())/(1024*1024))

	// Verify the created image
	fmt.Println("\nStep 4: Verifying created image...")
	if err := VerifyCPIO(outputPath); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	fmt.Printf("\n✅ Successfully created initramfs: %s\n", outputPath)
	fmt.Println("\nYou can now boot with:")
	fmt.Printf("  qemu-system-x86_64 -kernel vmlinuz -initrd %s\n", outputPath)
	return nil
}

// ExtractCPIO extracts a CPIO archive for inspection
func ExtractCPIO(imagePath string) error {
	fmt.Printf("Extracting CPIO archive: %s\n", imagePath)

	// Create extraction directory
	extractDir := strings.TrimSuffix(imagePath, ".cpio.gz") + "_extracted"
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extract directory: %w", err)
	}

	// Decompress if gzipped
	var cpioReader io.Reader
	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	if strings.HasSuffix(imagePath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		cpioReader = gzReader
	} else {
		cpioReader = file
	}

	// Write to temp file for cpio extraction
	tempCpio := filepath.Join(extractDir, "temp.cpio")
	tempFile, err := os.Create(tempCpio)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, cpioReader); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to copy cpio data: %w", err)
	}
	tempFile.Close()

	// Extract using cpio command
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("cd %s && cpio -i -d < temp.cpio 2>/dev/null", extractDir))

	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tempCpio)
		return fmt.Errorf("failed to extract cpio: %v\nOutput: %s", err, output)
	}

	// Clean up temp file
	os.Remove(tempCpio)

	fmt.Printf("✅ Extracted to: %s\n", extractDir)

	// List critical files
	fmt.Println("\nCritical files found:")
	criticalPaths := []string{
		"sbin/init",
		"usr/bin/rock-manager",
		"usr/bin/volcano-agent",
		"bin/busybox",
		"bin/sh",
	}

	for _, path := range criticalPaths {
		fullPath := filepath.Join(extractDir, path)
		if info, err := os.Lstat(fullPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(fullPath)
				fmt.Printf("  ✓ %s -> %s\n", path, target)
			} else {
				fmt.Printf("  ✓ %s (%.2f MB)\n", path, float64(info.Size())/(1024*1024))
			}
		} else {
			fmt.Printf("  ✗ %s NOT FOUND\n", path)
		}
	}

	return nil
}

// VerifyCPIO verifies that a CPIO archive meets rock-init integration requirements
func VerifyCPIO(imagePath string) error {
	fmt.Printf("Verifying CPIO archive: %s\n", imagePath)
	fmt.Println("=" + strings.Repeat("=", 50))

	// Create temp directory for extraction
	tempDir, err := ioutil.TempDir("", "rock-verify-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract the archive
	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	var cpioReader io.Reader = file
	if strings.HasSuffix(imagePath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		cpioReader = gzReader
	}

	// Write to temp file and extract
	tempCpio := filepath.Join(tempDir, "temp.cpio")
	tempFile, err := os.Create(tempCpio)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, cpioReader); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to copy cpio data: %w", err)
	}
	tempFile.Close()

	// Extract using cpio
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("cd %s && cpio -i -d < temp.cpio 2>/dev/null", tempDir))

	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract for verification: %w", err)
	}

	// Verify structure
	var errors []string
	var warnings []string

	fmt.Println("\nChecking critical binaries...")
	for _, binary := range integration.RequiredBinaries {
		path := filepath.Join(tempDir, strings.TrimPrefix(binary.Destination, "/"))
		if info, err := os.Stat(path); err != nil {
			errors = append(errors, fmt.Sprintf("MISSING: %s", binary.Destination))
		} else {
			// Special check for rock-init -> init rename
			if binary.Source == "rock-init" && binary.Destination == "/sbin/init" {
				fmt.Printf("  ✅ %s (renamed from %s)\n", binary.Destination, binary.Source)
			} else {
				fmt.Printf("  ✅ %s (%.2f MB)\n", binary.Destination, float64(info.Size())/(1024*1024))
			}
		}
	}

	fmt.Println("\nChecking busybox symlinks...")
	essentialSymlinks := []string{"sh", "ls", "cat", "echo", "mount", "umount"}
	for _, symlink := range essentialSymlinks {
		path := filepath.Join(tempDir, "bin", symlink)
		if info, err := os.Lstat(path); err != nil {
			if symlink == "sh" {
				errors = append(errors, fmt.Sprintf("CRITICAL: /bin/sh missing (shell required)"))
			} else {
				warnings = append(warnings, fmt.Sprintf("Missing symlink: /bin/%s", symlink))
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(path)
			if target == "busybox" || target == "/bin/busybox" {
				fmt.Printf("  ✅ /bin/%s -> busybox\n", symlink)
			} else {
				warnings = append(warnings, fmt.Sprintf("/bin/%s points to %s (expected busybox)", symlink, target))
			}
		}
	}

	fmt.Println("\nChecking required directories...")
	criticalDirs := []string{"/proc", "/sys", "/dev", "/tmp", "/sbin", "/bin", "/usr/bin", "/config"}
	for _, dir := range criticalDirs {
		path := filepath.Join(tempDir, strings.TrimPrefix(dir, "/"))
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			warnings = append(warnings, fmt.Sprintf("Missing directory: %s", dir))
		} else {
			fmt.Printf("  ✅ %s/\n", dir)
		}
	}

	// Print results
	fmt.Println("\n" + strings.Repeat("=", 50))
	if len(errors) == 0 {
		fmt.Println("✅ INTEGRATION VERIFICATION PASSED")
		fmt.Println("This image should boot with rock-init!")
	} else {
		fmt.Println("❌ INTEGRATION VERIFICATION FAILED")
		fmt.Println("\nCritical Errors:")
		for _, err := range errors {
			fmt.Printf("  ❌ %s\n", err)
		}
		fmt.Println("\n⚠️  This image will NOT boot with rock-init!")
	}

	if len(warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warn := range warnings {
			fmt.Printf("  ⚠️  %s\n", warn)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("verification failed with %d errors", len(errors))
	}

	return nil
}

// verifyRootfsStructure checks if rootfs has required files before creating CPIO
func verifyRootfsStructure(rootfsPath string) error {
	var errors []string

	// Check critical binaries
	for _, binary := range integration.RequiredBinaries {
		path := filepath.Join(rootfsPath, strings.TrimPrefix(binary.Destination, "/"))
		if _, err := os.Stat(path); err != nil {
			// Special message for rock-init
			if binary.Source == "rock-init" {
				errors = append(errors, fmt.Sprintf("%s not found (rock-init must be renamed to /sbin/init!)", binary.Destination))
			} else {
				errors = append(errors, fmt.Sprintf("%s not found", binary.Destination))
			}
		}
	}

	// Check shell symlink
	shellPath := filepath.Join(rootfsPath, "bin/sh")
	if info, err := os.Lstat(shellPath); err != nil {
		errors = append(errors, "/bin/sh not found (shell required)")
	} else if info.Mode()&os.ModeSymlink == 0 {
		// It exists but not a symlink - still might work but warn
		fmt.Printf("  ⚠️  /bin/sh exists but is not a symlink to busybox\n")
	}

	if len(errors) > 0 {
		fmt.Println("❌ Rootfs structure errors:")
		for _, err := range errors {
			fmt.Printf("  • %s\n", err)
		}
		return fmt.Errorf("rootfs does not meet requirements")
	}

	return nil
}

// ShowRequiredStructure displays the required initramfs structure
func ShowRequiredStructure() {
	fmt.Println("REQUIRED INITRAMFS STRUCTURE FOR ROCK-INIT")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("Based on rock-builder.yaml configuration")
	fmt.Println()

	fmt.Println("CRITICAL PATHS (hardcoded in rock-init):")
	fmt.Println("-----------------------------------------")
	fmt.Printf("  %-30s %s\n", "rock-init", "→ /sbin/init")
	fmt.Printf("  %-30s %s\n", "rock-manager", "→ /usr/bin/rock-manager")
	fmt.Printf("  %-30s %s\n", "volcano-agent", "→ /usr/bin/volcano-agent")
	fmt.Printf("  %-30s %s\n", "busybox", "→ /bin/busybox")
	fmt.Printf("  %-30s %s\n", "shell", "→ /bin/sh (symlink to busybox)")
	fmt.Println()

	fmt.Println("Required Busybox Symlinks in /bin:")
	fmt.Println("-----------------------------------")
	symlinks := []string{
		"sh", "ls", "cat", "ps", "echo", "sleep",
		"mount", "umount", "mkdir", "rm", "cp", "mv",
		"grep", "awk", "sed", "find", "ifconfig", "route",
		"top", "tail", "head", "dmesg", "kill", "free", "df",
		"ping", "netstat", "wget", "nc", "ip", "vi",
	}
	for i := 0; i < len(symlinks); i += 6 {
		line := ""
		for j := i; j < i+6 && j < len(symlinks); j++ {
			line += fmt.Sprintf("%-12s", symlinks[j])
		}
		fmt.Printf("  %s\n", line)
	}
	fmt.Println()

	fmt.Println("Required Directories:")
	fmt.Println("--------------------")
	dirs := []string{
		"/bin", "/sbin", "/etc", "/dev", "/proc", "/sys",
		"/tmp", "/var/run", "/usr/bin", "/lib", "/lib64",
		"/config", "/etc/rock/tls",
	}
	for _, dir := range dirs {
		fmt.Printf("  %s/\n", dir)
	}
	fmt.Println()

	fmt.Println("Required Device Nodes:")
	fmt.Println("---------------------")
	for _, node := range integration.RequiredDeviceNodes {
		fmt.Printf("  %-20s (major:%d minor:%d mode:%04o)\n",
			node.Path, node.Major, node.Minor, node.Mode)
	}
	fmt.Println()

	fmt.Println("Example Rootfs Preparation:")
	fmt.Println("---------------------------")
	fmt.Println("  # Create directory structure")
	fmt.Println("  mkdir -p rootfs/{bin,sbin,etc,dev,proc,sys,tmp,var/run,usr/bin}")
	fmt.Println("  mkdir -p rootfs/{lib,lib64,config,etc/rock/tls}")
	fmt.Println()
	fmt.Println("  # Copy binaries (CRITICAL: rename rock-init!)")
	fmt.Println("  cp rock-init rootfs/sbin/init  # ← MUST RENAME!")
	fmt.Println("  cp rock-manager rootfs/usr/bin/")
	fmt.Println("  cp volcano-agent rootfs/usr/bin/")
	fmt.Println("  cp busybox rootfs/bin/")
	fmt.Println()
	fmt.Println("  # Create busybox symlinks")
	fmt.Println("  cd rootfs/bin")
	fmt.Println("  for cmd in sh ls cat echo mount umount mkdir rm; do")
	fmt.Println("    ln -s busybox $cmd")
	fmt.Println("  done")
	fmt.Println("  cd ../..")
	fmt.Println()
	fmt.Println("  # Create device nodes (requires root on Linux)")
	fmt.Println("  sudo mknod rootfs/dev/null c 1 3")
	fmt.Println("  sudo mknod rootfs/dev/console c 5 1")
	fmt.Println("  # ... etc")
	fmt.Println()
	fmt.Println("  # Create CPIO archive")
	fmt.Println("  rock-image cpio create rootfs")
	fmt.Println()
	fmt.Println("  # Verify the image")
	fmt.Println("  rock-image cpio verify initrd.cpio.gz")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
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

	case "cpio":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: missing cpio subcommand")
			printUsage()
			os.Exit(1)
		}

		subcommand := os.Args[2]
		switch subcommand {
		case "create":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: missing rootfs directory")
				fmt.Fprintln(os.Stderr, "Usage: rock-image cpio create <rootfs-dir>")
				os.Exit(1)
			}
			if err := CreateCPIO(os.Args[3]); err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				os.Exit(1)
			}

		case "extract":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: missing image path")
				fmt.Fprintln(os.Stderr, "Usage: rock-image cpio extract <image.cpio.gz>")
				os.Exit(1)
			}
			if err := ExtractCPIO(os.Args[3]); err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				os.Exit(1)
			}

		case "verify":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: missing image path")
				fmt.Fprintln(os.Stderr, "Usage: rock-image cpio verify <image.cpio.gz>")
				os.Exit(1)
			}
			if err := VerifyCPIO(os.Args[3]); err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				os.Exit(1)
			}

		default:
			fmt.Fprintf(os.Stderr, "Error: unknown cpio subcommand: %s\n", subcommand)
			printUsage()
			os.Exit(1)
		}

	default:
		// Legacy commands for backward compatibility
		switch command {
		case "create", "validate":
			fmt.Fprintln(os.Stderr, "Note: Please use the new cpio subcommands:")
			fmt.Fprintln(os.Stderr, "  rock-image cpio create <rootfs-dir>")
			fmt.Fprintln(os.Stderr, "  rock-image cpio verify <image.cpio.gz>")
			os.Exit(1)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Println("rock-image - CRITICAL Image Creation Tool for ROCK-OS")
	fmt.Println()
	fmt.Println("This tool creates initramfs images with the EXACT structure")
	fmt.Println("required by rock-init. Getting paths wrong = no boot!")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  rock-image cpio create <rootfs-dir>     Create CPIO initramfs")
	fmt.Println("  rock-image cpio extract <image.cpio.gz> Extract for inspection")
	fmt.Println("  rock-image cpio verify <image.cpio.gz>  Verify integration")
	fmt.Println("  rock-image structure                    Show required structure")
	fmt.Println("  rock-image version                      Show version")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Prepare rootfs directory")
	fmt.Println("  mkdir -p rootfs/{sbin,bin,usr/bin,dev,proc,sys}")
	fmt.Println("  cp rock-init rootfs/sbin/init  # MUST RENAME!")
	fmt.Println("  cp rock-manager rootfs/usr/bin/")
	fmt.Println("  cp volcano-agent rootfs/usr/bin/")
	fmt.Println("  cp busybox rootfs/bin/")
	fmt.Println("  cd rootfs/bin && ln -s busybox sh && cd ../..")
	fmt.Println()
	fmt.Println("  # Create and verify image")
	fmt.Println("  rock-image cpio create rootfs")
	fmt.Println("  rock-image cpio verify initrd.cpio.gz")
	fmt.Println()
	fmt.Println("CRITICAL INTEGRATION PATHS:")
	fmt.Println("  • rock-init MUST be at /sbin/init (renamed!)")
	fmt.Println("  • rock-manager MUST be at /usr/bin/rock-manager")
	fmt.Println("  • volcano-agent MUST be at /usr/bin/volcano-agent")
	fmt.Println("  • Shell MUST be at /bin/sh (symlink to busybox)")
}