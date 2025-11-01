package integration

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// VerificationError contains details about a verification failure
type VerificationError struct {
	Path    string
	Reason  string
	Details string
}

func (e VerificationError) Error() string {
	return fmt.Sprintf("INTEGRATION FAIL: %s - %s", e.Path, e.Reason)
}

// VerificationResult contains the results of an integration verification
type VerificationResult struct {
	Success bool
	Errors  []VerificationError
	Warnings []string
}

// VerifyImage verifies that an initramfs image meets rock-init integration requirements
func VerifyImage(imagePath string) (*VerificationResult, error) {
	result := &VerificationResult{Success: true}

	// Open the image file
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	// Determine if it's compressed
	var reader io.Reader = file
	if strings.HasSuffix(imagePath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// Create tar reader if it's a tar archive
	var files = make(map[string]bool)
	if strings.Contains(imagePath, ".tar") || strings.Contains(imagePath, ".cpio") {
		// For cpio, we'd need a different reader, but for now assume tar
		tarReader := tar.NewReader(reader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read archive: %w", err)
			}
			files[header.Name] = true

			// Normalize path (remove leading ./)
			normalizedPath := strings.TrimPrefix(header.Name, ".")
			files[normalizedPath] = true
		}
	}

	// Check required binaries
	for _, binary := range RequiredBinaries {
		path := binary.Destination
		if !checkPathExists(files, path) {
			result.Success = false
			result.Errors = append(result.Errors, VerificationError{
				Path:    path,
				Reason:  fmt.Sprintf("%s must be at this exact location", binary.Source),
				Details: "This path is hardcoded in rock-init",
			})
		}
	}

	// Check busybox symlinks
	for _, symlink := range BusyboxSymlinks {
		path := filepath.Join("/bin", symlink)
		if !checkPathExists(files, path) {
			// This is a warning, not an error
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Missing busybox symlink: %s", path))
		}
	}

	// Special check for shell
	if !checkPathExists(files, ShellPath) {
		result.Success = false
		result.Errors = append(result.Errors, VerificationError{
			Path:    ShellPath,
			Reason:  "Shell is required for rock-init",
			Details: "Must be a symlink to busybox",
		})
	}

	return result, nil
}

// VerifyRootfs verifies a rootfs directory structure
func VerifyRootfs(rootfsPath string) (*VerificationResult, error) {
	result := &VerificationResult{Success: true}

	// Check required binaries
	for _, binary := range RequiredBinaries {
		fullPath := filepath.Join(rootfsPath, binary.Destination)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			result.Success = false
			result.Errors = append(result.Errors, VerificationError{
				Path:    binary.Destination,
				Reason:  fmt.Sprintf("%s must be at this exact location", binary.Source),
				Details: "This path is hardcoded in rock-init",
			})
		}
	}

	// Check shell symlink
	shellPath := filepath.Join(rootfsPath, ShellPath)
	if _, err := os.Stat(shellPath); os.IsNotExist(err) {
		result.Success = false
		result.Errors = append(result.Errors, VerificationError{
			Path:    ShellPath,
			Reason:  "Shell is required for rock-init",
			Details: "Must be a symlink to busybox",
		})
	}

	// Check required directories
	for _, dir := range RequiredDirectories {
		fullPath := filepath.Join(rootfsPath, dir)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Missing directory: %s", dir))
		}
	}

	// Check device nodes
	devPath := filepath.Join(rootfsPath, "/dev")
	if info, err := os.Stat(devPath); err == nil && info.IsDir() {
		for _, node := range RequiredDeviceNodes {
			nodePath := filepath.Join(rootfsPath, node.Path)
			if _, err := os.Stat(nodePath); os.IsNotExist(err) {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Missing device node: %s", node.Path))
			}
		}
	}

	return result, nil
}

// checkPathExists checks if a path exists in the file map
func checkPathExists(files map[string]bool, path string) bool {
	// Check exact path
	if files[path] {
		return true
	}

	// Check without leading slash
	if files[strings.TrimPrefix(path, "/")] {
		return true
	}

	// Check with leading ./
	if files["." + path] {
		return true
	}

	return false
}

// PrintVerificationResult prints the verification result in a formatted way
func PrintVerificationResult(result *VerificationResult) {
	if result.Success {
		fmt.Println("✅ INTEGRATION VERIFICATION PASSED")
	} else {
		fmt.Println("❌ INTEGRATION VERIFICATION FAILED")
		fmt.Println("\nCritical Errors:")
		for _, err := range result.Errors {
			fmt.Printf("  ❌ %s\n", err.Path)
			fmt.Printf("     Reason: %s\n", err.Reason)
			if err.Details != "" {
				fmt.Printf("     Details: %s\n", err.Details)
			}
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warning := range result.Warnings {
			fmt.Printf("  ⚠️  %s\n", warning)
		}
	}
}