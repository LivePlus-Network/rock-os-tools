// rock-kernel - Alpine Linux Kernel Manager for ROCK-OS
//
// This tool solves the immediate problem of kernel management.
// Start here - this can be built and used TODAY.
//
// Usage:
//   rock-kernel fetch alpine:5.10.186
//   rock-kernel extract vmlinuz-5.10.186.apk
//   rock-kernel list
//   rock-kernel verify vmlinuz --checksum sha256:abc123...
//
// Build:
//   go build -o rock-kernel rock-kernel-starter.go
//
// This will later become pkg/kernel library for rock-os-image-server

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock-os/tools/pkg/integration"
)

// KernelSpec represents a kernel specification
type KernelSpec struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Arch     string `json:"arch"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
}

// KernelInfo represents cached kernel information
type KernelInfo struct {
	Spec       KernelSpec `json:"spec"`
	Path       string     `json:"path"`
	CachedAt   time.Time  `json:"cached_at"`
	Extracted  bool       `json:"extracted"`
	VmlinuzPath string    `json:"vmlinuz_path,omitempty"`
}

// KernelManager manages kernel downloads and caching
type KernelManager struct {
	CacheDir string
	Registry map[string]KernelSpec
}

// NewKernelManager creates a new kernel manager
func NewKernelManager() *KernelManager {
	cacheDir := os.Getenv("ROCK_KERNEL_CACHE")
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".rock", "kernels")
	}

	// Ensure cache directory exists
	os.MkdirAll(cacheDir, 0755)

	return &KernelManager{
		CacheDir: cacheDir,
		Registry: getDefaultRegistry(),
	}
}

// getDefaultRegistry returns the default kernel registry
func getDefaultRegistry() map[string]KernelSpec {
	return map[string]KernelSpec{
		"alpine:5.10.180": {
			Name:     "alpine",
			Version:  "5.10.180",
			Arch:     "x86_64",
			URL:      "https://dl-cdn.alpinelinux.org/alpine/v3.14/main/x86_64/linux-lts-5.10.180-r0.apk",
			Checksum: "sha256:1234567890abcdef", // TODO: Add real checksum
		},
		"alpine:6.1.140": {
			Name:     "alpine",
			Version:  "6.1.140",
			Arch:     "x86_64",
			URL:      "https://dl-cdn.alpinelinux.org/alpine/v3.18/main/x86_64/linux-lts-6.1.140-r0.apk",
			Checksum: "sha256:1234567890abcdef", // TODO: Add real checksum
		},
		"alpine:5.10.180-hardened": {
			Name:     "alpine-hardened",
			Version:  "5.10.180",
			Arch:     "x86_64",
			URL:      "https://dl-cdn.alpinelinux.org/alpine/v3.14/main/x86_64/linux-hardened-5.10.180-r0.apk",
			Checksum: "sha256:fedcba0987654321", // TODO: Add real checksum
		},
		"alpine:latest": {
			Name:     "alpine",
			Version:  "6.1.66", // Update to latest
			Arch:     "x86_64",
			URL:      "https://dl-cdn.alpinelinux.org/alpine/v3.19/main/x86_64/linux-lts-6.1.66-r0.apk",
			Checksum: "sha256:abcdef1234567890", // TODO: Add real checksum
		},
	}
}

// Fetch downloads a kernel by specification
func (km *KernelManager) Fetch(spec string) (*KernelInfo, error) {
	// Parse specification (e.g., "alpine:5.10.186")
	kernelSpec, exists := km.Registry[spec]
	if !exists {
		return nil, fmt.Errorf("kernel spec not found: %s", spec)
	}

	// Check if already cached
	cachedPath := filepath.Join(km.CacheDir, fmt.Sprintf("%s-%s.apk", kernelSpec.Name, kernelSpec.Version))
	if _, err := os.Stat(cachedPath); err == nil {
		fmt.Printf("Using cached kernel: %s\n", cachedPath)
		return &KernelInfo{
			Spec:     kernelSpec,
			Path:     cachedPath,
			CachedAt: time.Now(),
		}, nil
	}

	// Download kernel
	fmt.Printf("Downloading kernel from: %s\n", kernelSpec.URL)
	resp, err := http.Get(kernelSpec.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// Create temporary file
	tmpFile, err := os.CreateTemp(km.CacheDir, "kernel-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Copy with progress
	size := resp.ContentLength
	reader := &progressReader{
		Reader: resp.Body,
		Total:  size,
	}

	hash := sha256.New()
	writer := io.MultiWriter(tmpFile, hash)

	_, err = io.Copy(writer, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	tmpFile.Close()

	// Verify checksum (if provided)
	actualChecksum := fmt.Sprintf("sha256:%s", hex.EncodeToString(hash.Sum(nil)))
	if kernelSpec.Checksum != "" && !strings.HasPrefix(kernelSpec.Checksum, "sha256:1234") { // Skip placeholder
		if actualChecksum != kernelSpec.Checksum {
			return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", kernelSpec.Checksum, actualChecksum)
		}
		fmt.Println("✓ Checksum verified")
	} else {
		fmt.Printf("⚠️  Checksum not verified (got %s)\n", actualChecksum)
	}

	// Move to final location
	if err := os.Rename(tmpFile.Name(), cachedPath); err != nil {
		return nil, fmt.Errorf("failed to save kernel: %w", err)
	}

	fmt.Printf("✓ Kernel saved to: %s\n", cachedPath)

	return &KernelInfo{
		Spec:     kernelSpec,
		Path:     cachedPath,
		CachedAt: time.Now(),
	}, nil
}

// Extract extracts vmlinuz from APK package
func (km *KernelManager) Extract(apkPath string) (*KernelInfo, error) {
	fmt.Printf("Extracting kernel from: %s\n", apkPath)

	// Open APK file (it's a tar.gz)
	file, err := os.Open(apkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open APK: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract directory
	extractDir := strings.TrimSuffix(apkPath, filepath.Ext(apkPath))
	os.MkdirAll(extractDir, 0755)

	var vmlinuzPath string

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar: %w", err)
		}

		// Look for vmlinuz
		if strings.Contains(header.Name, "vmlinuz") {
			targetPath := filepath.Join(extractDir, filepath.Base(header.Name))

			outFile, err := os.Create(targetPath)
			if err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return nil, fmt.Errorf("failed to extract file: %w", err)
			}
			outFile.Close()

			// Set permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				return nil, fmt.Errorf("failed to set permissions: %w", err)
			}

			vmlinuzPath = targetPath
			fmt.Printf("✓ Extracted vmlinuz to: %s\n", targetPath)
		}

		// Also extract modules if present
		if strings.Contains(header.Name, "modules") || strings.Contains(header.Name, ".ko") {
			targetPath := filepath.Join(extractDir, header.Name)
			os.MkdirAll(filepath.Dir(targetPath), 0755)

			if header.Typeflag == tar.TypeReg {
				outFile, err := os.Create(targetPath)
				if err != nil {
					continue
				}
				io.Copy(outFile, tarReader)
				outFile.Close()
			}
		}
	}

	if vmlinuzPath == "" {
		return nil, fmt.Errorf("vmlinuz not found in APK")
	}

	// Copy vmlinuz to standard location
	standardPath := filepath.Join(km.CacheDir, "vmlinuz")
	if err := copyFile(vmlinuzPath, standardPath); err != nil {
		return nil, fmt.Errorf("failed to copy vmlinuz: %w", err)
	}
	fmt.Printf("✓ Copied vmlinuz to: %s\n", standardPath)

	return &KernelInfo{
		Path:        apkPath,
		Extracted:   true,
		VmlinuzPath: standardPath,
		CachedAt:    time.Now(),
	}, nil
}

// List lists cached kernels
func (km *KernelManager) List() ([]KernelInfo, error) {
	var kernels []KernelInfo

	entries, err := os.ReadDir(km.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache dir: %w", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".apk") {
			info, _ := entry.Info()
			kernels = append(kernels, KernelInfo{
				Path:     filepath.Join(km.CacheDir, entry.Name()),
				CachedAt: info.ModTime(),
			})
		}
	}

	return kernels, nil
}

// progressReader wraps io.Reader to show progress
type progressReader struct {
	io.Reader
	Total   int64
	Current int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Current += int64(n)

	if pr.Total > 0 {
		percent := float64(pr.Current) * 100 / float64(pr.Total)
		fmt.Printf("\rDownloading: %.1f%% (%d/%d bytes)", percent, pr.Current, pr.Total)

		if pr.Current >= pr.Total {
			fmt.Println() // New line when complete
		}
	}

	return n, err
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// CLI Commands

func cmdFetch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rock-kernel fetch <spec>")
	}

	km := NewKernelManager()
	info, err := km.Fetch(args[0])
	if err != nil {
		return err
	}

	// Output JSON for scripting
	if os.Getenv("ROCK_OUTPUT") == "json" {
		data, _ := json.Marshal(info)
		fmt.Println(string(data))
	}

	return nil
}

func cmdExtract(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rock-kernel extract <apk-file>")
	}

	km := NewKernelManager()
	info, err := km.Extract(args[0])
	if err != nil {
		return err
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		data, _ := json.Marshal(info)
		fmt.Println(string(data))
	}

	return nil
}

func cmdList(args []string) error {
	km := NewKernelManager()
	kernels, err := km.List()
	if err != nil {
		return err
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		data, _ := json.Marshal(kernels)
		fmt.Println(string(data))
	} else {
		fmt.Println("Cached kernels:")
		for _, k := range kernels {
			fmt.Printf("  - %s (cached %s)\n", filepath.Base(k.Path), k.CachedAt.Format("2006-01-02 15:04"))
		}
	}

	return nil
}

func cmdCmdline(args []string) error {
	mode := "debug"
	if len(args) > 0 {
		mode = args[0]
	}

	// Use the integration package to get the correct cmdline
	// This ensures we ALWAYS use the correct init path
	cmdline := integration.GetKernelCmdline(mode)

	// Validate the cmdline to ensure it's correct
	if err := integration.ValidateKernelCmdline(cmdline); err != nil {
		return fmt.Errorf("invalid kernel cmdline: %w", err)
	}

	fmt.Println(cmdline)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("rock-kernel - Alpine Linux Kernel Manager for ROCK-OS")
		fmt.Println("\nUsage:")
		fmt.Println("  rock-kernel fetch <spec>     Download kernel (e.g., alpine:5.10.186)")
		fmt.Println("  rock-kernel extract <apk>    Extract vmlinuz from APK")
		fmt.Println("  rock-kernel list             List cached kernels")
		fmt.Println("  rock-kernel cmdline [mode]   Get kernel command line")
		fmt.Println("\nEnvironment:")
		fmt.Println("  ROCK_KERNEL_CACHE  Cache directory (default: ~/.rock/kernels)")
		fmt.Println("  ROCK_OUTPUT=json   Output JSON for scripting")
		os.Exit(1)
	}

	var err error
	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "fetch":
		err = cmdFetch(args)
	case "extract":
		err = cmdExtract(args)
	case "list":
		err = cmdList(args)
	case "cmdline":
		err = cmdCmdline(args)
	default:
		err = fmt.Errorf("unknown command: %s", command)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}