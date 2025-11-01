package main

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Dependency represents a shared library dependency
type Dependency struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Found    bool   `json:"found"`
	Size     int64  `json:"size,omitempty"`
	RealPath string `json:"real_path,omitempty"` // After resolving symlinks
}

// ScanResult contains all dependency information
type ScanResult struct {
	Binary       string       `json:"binary"`
	Architecture string       `json:"architecture"`
	Dependencies []Dependency `json:"dependencies"`
	TotalSize    int64        `json:"total_size"`
	IsMusl       bool         `json:"is_musl"`
	IsStatic     bool         `json:"is_static"`
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "scan":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: scan requires a binary path\n")
			os.Exit(1)
		}
		cmdScan(os.Args[2])

	case "copy":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: copy requires binary and destination\n")
			os.Exit(1)
		}
		cmdCopy(os.Args[2], os.Args[3])

	case "verify":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: verify requires a binary path\n")
			os.Exit(1)
		}
		cmdVerify(os.Args[2])

	case "check":
		// Quick check for a specific library
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: check requires binary and library name\n")
			os.Exit(1)
		}
		cmdCheck(os.Args[2], os.Args[3])

	case "alpine":
		// Special handling for Alpine Linux binaries
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: alpine requires a binary path\n")
			os.Exit(1)
		}
		cmdAlpine(os.Args[2])

	case "version":
		fmt.Printf("rock-deps version %s (built %s, commit %s)\n",
			Version, BuildTime, GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-deps - Binary Dependency Scanner for ROCK-OS

This tool scans ELF binaries to find all shared library dependencies.
Critical for ensuring initramfs contains all required libraries.

Usage:
  rock-deps scan <binary>           Scan and list all dependencies
  rock-deps copy <binary> <dest>    Copy binary with all dependencies
  rock-deps verify <binary>         Verify all dependencies are available
  rock-deps check <binary> <lib>    Check if binary needs specific library
  rock-deps alpine <binary>         Alpine/musl-specific analysis
  rock-deps version                 Show version information

Examples:
  # Scan rock-init for dependencies
  rock-deps scan /path/to/rock-init

  # Copy volcano-agent with all libraries
  rock-deps copy volcano-agent ./rootfs/usr/bin/

  # Verify binary has all dependencies
  rock-deps verify ./rootfs/usr/bin/rock-manager

  # Check for musl dependencies (Alpine)
  rock-deps alpine ./alpine-binary

Environment:
  ROCK_OUTPUT=json    Output in JSON format
  ROCK_VERBOSE=1      Show detailed information
  ROCK_SYSROOT=/path  Alternative sysroot for libraries

Notes:
  • Handles both glibc and musl libc binaries
  • Detects statically linked binaries
  • Resolves symlinks to find real library files
  • Critical for Alpine Linux compatibility`)
}

func cmdScan(binaryPath string) {
	result, err := scanBinary(binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", binaryPath, err)
		os.Exit(1)
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(result)
		return
	}

	// Human-readable output
	fmt.Printf("Scanning: %s\n", result.Binary)
	fmt.Printf("Architecture: %s\n", result.Architecture)

	if result.IsStatic {
		fmt.Println("Type: Statically linked (no dependencies)")
		return
	}

	if result.IsMusl {
		fmt.Println("Libc: musl (Alpine Linux)")
	} else {
		fmt.Println("Libc: glibc (standard)")
	}

	fmt.Printf("\nDependencies (%d):\n", len(result.Dependencies))
	fmt.Println("=" + strings.Repeat("=", 60))

	var totalSize int64
	for _, dep := range result.Dependencies {
		status := "✅"
		if !dep.Found {
			status = "❌"
		}

		if dep.Path != "" {
			sizeStr := formatSize(dep.Size)
			fmt.Printf("%s %-30s %s (%s)\n", status, dep.Name, dep.Path, sizeStr)
			if dep.RealPath != "" && dep.RealPath != dep.Path {
				fmt.Printf("     → %s\n", dep.RealPath)
			}
			totalSize += dep.Size
		} else {
			fmt.Printf("%s %-30s NOT FOUND\n", status, dep.Name)
		}
	}

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("Total size with dependencies: %s\n", formatSize(totalSize))

	// Check for missing dependencies
	missing := 0
	for _, dep := range result.Dependencies {
		if !dep.Found {
			missing++
		}
	}

	if missing > 0 {
		fmt.Printf("\n⚠️  WARNING: %d dependencies not found!\n", missing)
		os.Exit(1)
	}
}

func cmdCopy(binaryPath, destDir string) {
	// Scan the binary first
	result, err := scanBinary(binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", binaryPath, err)
		os.Exit(1)
	}

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", destDir, err)
		os.Exit(1)
	}

	// Copy the binary itself
	destBinary := filepath.Join(destDir, filepath.Base(binaryPath))
	if err := copyFile(binaryPath, destBinary); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying binary: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Copied binary: %s\n", destBinary)

	if result.IsStatic {
		fmt.Println("Binary is statically linked - no dependencies to copy")
		return
	}

	// Create lib directories
	libDir := filepath.Join(destDir, "..", "..", "lib")
	lib64Dir := filepath.Join(destDir, "..", "..", "lib64")
	usrLibDir := filepath.Join(destDir, "..", "..", "usr", "lib")

	// Copy each dependency
	copied := 0
	for _, dep := range result.Dependencies {
		if !dep.Found {
			fmt.Printf("⚠️  Skipping missing dependency: %s\n", dep.Name)
			continue
		}

		// Determine destination based on original path
		var destPath string
		switch {
		case strings.HasPrefix(dep.Path, "/lib64/"):
			os.MkdirAll(lib64Dir, 0755)
			destPath = filepath.Join(lib64Dir, filepath.Base(dep.Path))
		case strings.HasPrefix(dep.Path, "/usr/lib/"):
			os.MkdirAll(usrLibDir, 0755)
			destPath = filepath.Join(usrLibDir, filepath.Base(dep.Path))
		default:
			os.MkdirAll(libDir, 0755)
			destPath = filepath.Join(libDir, filepath.Base(dep.Path))
		}

		// Copy the actual file (resolve symlinks)
		sourcePath := dep.RealPath
		if sourcePath == "" {
			sourcePath = dep.Path
		}

		if err := copyFile(sourcePath, destPath); err != nil {
			fmt.Printf("❌ Failed to copy %s: %v\n", dep.Name, err)
		} else {
			fmt.Printf("✅ Copied dependency: %s → %s\n", dep.Name, destPath)
			copied++
		}

		// If original was a symlink, create the symlink too
		if dep.RealPath != "" && dep.RealPath != dep.Path {
			linkName := filepath.Join(filepath.Dir(destPath), filepath.Base(dep.Path))
			os.Remove(linkName) // Remove if exists
			if err := os.Symlink(filepath.Base(dep.RealPath), linkName); err != nil {
				fmt.Printf("⚠️  Failed to create symlink %s: %v\n", linkName, err)
			}
		}
	}

	fmt.Printf("\n✅ Successfully copied binary with %d dependencies\n", copied)
	fmt.Printf("Total size: %s\n", formatSize(result.TotalSize))
}

func cmdVerify(binaryPath string) {
	result, err := scanBinary(binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", binaryPath, err)
		os.Exit(1)
	}

	fmt.Printf("Verifying: %s\n", binaryPath)
	fmt.Println("=" + strings.Repeat("=", 60))

	if result.IsStatic {
		fmt.Println("✅ Binary is statically linked - no dependencies needed")
		return
	}

	allFound := true
	for _, dep := range result.Dependencies {
		if dep.Found {
			fmt.Printf("✅ %s\n", dep.Name)
		} else {
			fmt.Printf("❌ %s - MISSING\n", dep.Name)
			allFound = false
		}
	}

	fmt.Println("=" + strings.Repeat("=", 60))

	if allFound {
		fmt.Printf("✅ All %d dependencies found\n", len(result.Dependencies))
	} else {
		missing := 0
		for _, dep := range result.Dependencies {
			if !dep.Found {
				missing++
			}
		}
		fmt.Printf("❌ Missing %d of %d dependencies\n", missing, len(result.Dependencies))
		os.Exit(1)
	}
}

func cmdCheck(binaryPath, libName string) {
	result, err := scanBinary(binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", binaryPath, err)
		os.Exit(1)
	}

	for _, dep := range result.Dependencies {
		if strings.Contains(dep.Name, libName) {
			if dep.Found {
				fmt.Printf("✅ %s requires %s (found at %s)\n",
					filepath.Base(binaryPath), dep.Name, dep.Path)
			} else {
				fmt.Printf("❌ %s requires %s (NOT FOUND)\n",
					filepath.Base(binaryPath), dep.Name)
				os.Exit(1)
			}
			return
		}
	}

	fmt.Printf("ℹ️  %s does not require %s\n", filepath.Base(binaryPath), libName)
}

func cmdAlpine(binaryPath string) {
	result, err := scanBinary(binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", binaryPath, err)
		os.Exit(1)
	}

	fmt.Printf("Alpine/musl Analysis: %s\n", binaryPath)
	fmt.Println("=" + strings.Repeat("=", 60))

	if result.IsStatic {
		fmt.Println("✅ Statically linked - perfect for Alpine!")
		fmt.Println("   No runtime dependencies needed")
		return
	}

	if result.IsMusl {
		fmt.Println("✅ musl-linked binary (Alpine native)")
		fmt.Println("\nRequired musl libraries:")
		for _, dep := range result.Dependencies {
			if strings.Contains(dep.Name, "musl") ||
			   strings.Contains(dep.Name, "libc.so") {
				if dep.Found {
					fmt.Printf("  ✅ %s\n", dep.Name)
				} else {
					fmt.Printf("  ❌ %s (needs Alpine environment)\n", dep.Name)
				}
			}
		}
	} else {
		fmt.Println("⚠️  glibc-linked binary")
		fmt.Println("   May NOT work on Alpine without glibc compatibility layer")
		fmt.Println("\nWould need glibc libraries:")
		for _, dep := range result.Dependencies {
			if strings.Contains(dep.Name, "libc.so") ||
			   strings.Contains(dep.Name, "ld-linux") {
				fmt.Printf("  • %s\n", dep.Name)
			}
		}
		fmt.Println("\nConsider:")
		fmt.Println("  1. Rebuilding with musl libc")
		fmt.Println("  2. Static linking (add -static flag)")
		fmt.Println("  3. Using Alpine's gcompat package")
	}

	// Check for other Alpine-specific issues
	fmt.Println("\nOther dependencies:")
	for _, dep := range result.Dependencies {
		if !strings.Contains(dep.Name, "musl") &&
		   !strings.Contains(dep.Name, "libc.so") &&
		   !strings.Contains(dep.Name, "ld-linux") {
			status := "✅"
			if !dep.Found {
				status = "⚠️"
			}
			fmt.Printf("  %s %s\n", status, dep.Name)
		}
	}

	// Specific checks for ROCK-OS components
	if strings.Contains(binaryPath, "rock-init") ||
	   strings.Contains(binaryPath, "rock-manager") ||
	   strings.Contains(binaryPath, "volcano-agent") {
		fmt.Println("\n🔍 ROCK-OS Component Detected!")
		fmt.Println("Checking critical dependencies for Alpine initramfs:")

		criticalLibs := []string{
			"libssl.so", "libcrypto.so",  // OpenSSL
			"libz.so",                     // zlib
			"libpthread.so",               // Threading
		}

		for _, lib := range criticalLibs {
			found := false
			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Name, lib) {
					found = true
					if dep.Found {
						fmt.Printf("  ✅ %s\n", lib)
					} else {
						fmt.Printf("  ⚠️  %s (install in Alpine)\n", lib)
					}
					break
				}
			}
			if !found {
				fmt.Printf("  ✅ %s (not required)\n", lib)
			}
		}
	}
}

// scanBinary scans an ELF binary for dependencies
func scanBinary(binaryPath string) (*ScanResult, error) {
	// Open the ELF file
	file, err := elf.Open(binaryPath)
	if err != nil {
		// Try using file command as fallback
		return scanWithFile(binaryPath)
	}
	defer file.Close()

	result := &ScanResult{
		Binary:       binaryPath,
		Architecture: getArchitecture(file.Machine),
		Dependencies: []Dependency{},
	}

	// Check if statically linked
	_, err = file.DynamicSymbols()
	if err != nil {
		result.IsStatic = true
		return result, nil
	}

	// Get dependencies from ELF headers
	deps := extractDependencies(file)

	// Try to use ldd if available (won't work on macOS for Linux binaries)
	if runtime.GOOS == "linux" {
		deps = scanWithLDD(binaryPath, deps)
	}

	// Check for musl
	for _, dep := range deps {
		if strings.Contains(dep.Name, "musl") ||
		   (strings.Contains(dep.Name, "libc.so") && !strings.Contains(dep.Name, "glibc")) {
			result.IsMusl = true
			break
		}
	}

	// Calculate total size
	stat, err := os.Stat(binaryPath)
	if err == nil {
		result.TotalSize = stat.Size()
		for _, dep := range deps {
			result.TotalSize += dep.Size
		}
	}

	result.Dependencies = deps
	return result, nil
}

// extractDependencies gets NEEDED entries from ELF
func extractDependencies(file *elf.File) []Dependency {
	deps := []Dependency{}

	// Get the dynamic section
	dynSection := file.Section(".dynamic")
	if dynSection == nil {
		return deps
	}

	// Parse for DT_NEEDED entries
	libs, _ := file.DynString(elf.DT_NEEDED)
	for _, lib := range libs {
		dep := Dependency{
			Name:  lib,
			Found: false,
		}

		// Try to find the library
		paths := getLibrarySearchPaths()
		for _, searchPath := range paths {
			libPath := filepath.Join(searchPath, lib)
			if stat, err := os.Stat(libPath); err == nil {
				dep.Path = libPath
				dep.Found = true
				dep.Size = stat.Size()

				// Resolve symlinks
				if real, err := filepath.EvalSymlinks(libPath); err == nil && real != libPath {
					dep.RealPath = real
					if stat, err := os.Stat(real); err == nil {
						dep.Size = stat.Size()
					}
				}
				break
			}
		}

		deps = append(deps, dep)
	}

	return deps
}

// scanWithLDD uses ldd command (Linux only)
func scanWithLDD(binaryPath string, existingDeps []Dependency) []Dependency {
	cmd := exec.Command("ldd", binaryPath)
	output, err := cmd.Output()
	if err != nil {
		return existingDeps
	}

	// Map existing deps by name for merging
	depMap := make(map[string]*Dependency)
	for i := range existingDeps {
		depMap[existingDeps[i].Name] = &existingDeps[i]
	}

	// Parse ldd output
	scanner := bufio.NewScanner(bytes.NewReader(output))
	lddRegex := regexp.MustCompile(`^\s*(\S+)\s*=>\s*(\S+)\s+\(0x[0-9a-f]+\)`)
	vdsoRegex := regexp.MustCompile(`^\s*(linux-vdso\.so\.\d+)\s+\(0x[0-9a-f]+\)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for vdso (virtual dynamic shared object)
		if matches := vdsoRegex.FindStringSubmatch(line); matches != nil {
			continue // Skip vdso
		}

		// Parse normal library entries
		if matches := lddRegex.FindStringSubmatch(line); matches != nil {
			libName := matches[1]
			libPath := matches[2]

			if existing, ok := depMap[libName]; ok {
				// Update existing entry
				if libPath != "not" && libPath != "" {
					existing.Path = libPath
					existing.Found = true

					if stat, err := os.Stat(libPath); err == nil {
						existing.Size = stat.Size()
					}

					if real, err := filepath.EvalSymlinks(libPath); err == nil && real != libPath {
						existing.RealPath = real
					}
				}
			} else {
				// Add new entry found by ldd
				dep := Dependency{
					Name:  libName,
					Path:  libPath,
					Found: libPath != "not" && libPath != "",
				}

				if dep.Found {
					if stat, err := os.Stat(libPath); err == nil {
						dep.Size = stat.Size()
					}

					if real, err := filepath.EvalSymlinks(libPath); err == nil && real != libPath {
						dep.RealPath = real
					}
				}

				existingDeps = append(existingDeps, dep)
			}
		}
	}

	return existingDeps
}

// scanWithFile uses file command as fallback
func scanWithFile(binaryPath string) (*ScanResult, error) {
	cmd := exec.Command("file", binaryPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("not an ELF binary or file command failed")
	}

	outputStr := string(output)
	result := &ScanResult{
		Binary:       binaryPath,
		Dependencies: []Dependency{},
	}

	// Check if it's an ELF file
	if !strings.Contains(outputStr, "ELF") {
		return nil, fmt.Errorf("not an ELF binary")
	}

	// Detect architecture
	if strings.Contains(outputStr, "x86-64") || strings.Contains(outputStr, "x86_64") {
		result.Architecture = "x86_64"
	} else if strings.Contains(outputStr, "ARM aarch64") {
		result.Architecture = "aarch64"
	} else if strings.Contains(outputStr, "32-bit") {
		result.Architecture = "i386"
	}

	// Check if statically linked
	if strings.Contains(outputStr, "statically linked") {
		result.IsStatic = true
	}

	return result, nil
}

// getArchitecture converts ELF machine type to string
func getArchitecture(machine elf.Machine) string {
	switch machine {
	case elf.EM_X86_64:
		return "x86_64"
	case elf.EM_386:
		return "i386"
	case elf.EM_AARCH64:
		return "aarch64"
	case elf.EM_ARM:
		return "arm"
	default:
		return machine.String()
	}
}

// getLibrarySearchPaths returns standard library search paths
func getLibrarySearchPaths() []string {
	paths := []string{
		"/lib",
		"/lib64",
		"/usr/lib",
		"/usr/lib64",
		"/usr/local/lib",
		"/usr/local/lib64",
	}

	// Add sysroot if specified
	if sysroot := os.Getenv("ROCK_SYSROOT"); sysroot != "" {
		sysPaths := []string{}
		for _, p := range paths {
			sysPaths = append(sysPaths, filepath.Join(sysroot, p))
		}
		paths = append(sysPaths, paths...)
	}

	// Add LD_LIBRARY_PATH entries
	if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
		paths = append(strings.Split(ldPath, ":"), paths...)
	}

	return paths
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Get source file info
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy contents
	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return nil
}

// formatSize formats bytes to human readable
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// outputJSON outputs result as JSON
func outputJSON(result *ScanResult) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(result)
}