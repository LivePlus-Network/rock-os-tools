package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Component represents a buildable ROCK-OS component
type Component struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	SourcePath string `json:"source_path"`
	BinaryName string `json:"binary_name"`
	OutputPath string `json:"output_path"`
	Language   string `json:"language"`
	Target     string `json:"target,omitempty"`
}

// BuildResult contains the result of a build operation
type BuildResult struct {
	Component   string        `json:"component"`
	Success     bool          `json:"success"`
	OutputPath  string        `json:"output_path,omitempty"`
	Size        int64         `json:"size,omitempty"`
	BuildTime   time.Duration `json:"build_time"`
	Error       string        `json:"error,omitempty"`
	IsStaticBin bool          `json:"is_static"`
	Target      string        `json:"target"`
}

// BuildConfig holds build configuration
type BuildConfig struct {
	Target      string            `json:"target"`
	Profile     string            `json:"profile"`
	Features    []string          `json:"features"`
	Environment map[string]string `json:"environment"`
	OutputDir   string            `json:"output_dir"`
	SourceRoot  string            `json:"source_root"`
	Verbose     bool              `json:"verbose"`
}

// Default configuration
var defaultConfig = BuildConfig{
	Target:     "x86_64-unknown-linux-musl",  // Alpine Linux compatible
	Profile:    "release",
	OutputDir:  "./output",
	SourceRoot: "../",  // Assume rock-os repos are siblings
}

// Component definitions
var components = map[string]Component{
	"init": {
		Name:       "rock-init",
		Repository: "rock-os/init",
		SourcePath: "rock-init",
		BinaryName: "rock-init",
		OutputPath: "sbin/init",  // MUST be renamed to init!
		Language:   "rust",
		Target:     "x86_64-unknown-linux-musl",
	},
	"manager": {
		Name:       "rock-manager",
		Repository: "rock-os/manager",
		SourcePath: "rock-manager",
		BinaryName: "rock-manager",
		OutputPath: "usr/bin/rock-manager",
		Language:   "rust",
		Target:     "x86_64-unknown-linux-musl",
	},
	"agent": {
		Name:       "volcano-agent",
		Repository: "rock-os/agent",
		SourcePath: "volcano-agent",
		BinaryName: "volcano-agent",
		OutputPath: "usr/bin/volcano-agent",
		Language:   "rust",
		Target:     "x86_64-unknown-linux-musl",
	},
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Parse config from environment
	config := loadConfig()

	switch command {
	case "init":
		buildComponent("init", config)

	case "manager":
		buildComponent("manager", config)

	case "agent":
		buildComponent("agent", config)

	case "all":
		buildAll(config)

	case "check":
		checkBuildEnvironment()

	case "setup":
		setupRustTarget(config.Target)

	case "clean":
		cleanBuildArtifacts(config)

	case "list":
		listComponents()

	case "version":
		fmt.Printf("rock-build version %s (built %s, commit %s)\n",
			Version, BuildTime, GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-build - ROCK-OS Component Builder

Builds ROCK-OS components with correct Rust targets for Alpine Linux.
Uses x86_64-unknown-linux-musl for static linking.

Usage:
  rock-build init              Build rock-init
  rock-build manager           Build rock-manager
  rock-build agent             Build volcano-agent
  rock-build all               Build all components
  rock-build check             Check build environment
  rock-build setup             Setup Rust target
  rock-build clean             Clean build artifacts
  rock-build list              List components
  rock-build version           Show version

Examples:
  # Build rock-init for Alpine
  rock-build init

  # Build all components
  rock-build all

  # Check if Rust is properly configured
  rock-build check

  # Setup musl target
  rock-build setup

Environment:
  ROCK_SOURCE_ROOT    Source directory root (default: ../)
  ROCK_OUTPUT_DIR     Output directory (default: ./output)
  ROCK_TARGET         Rust target (default: x86_64-unknown-linux-musl)
  ROCK_PROFILE        Build profile (release/debug, default: release)
  ROCK_FEATURES       Comma-separated features
  ROCK_VERBOSE=1      Verbose output
  ROCK_OUTPUT=json    JSON output format

Build Targets:
  x86_64-unknown-linux-musl    Alpine Linux (recommended)
  x86_64-unknown-linux-gnu     Standard Linux
  x86_64-alpine-linux-musl     Alpine-specific

CRITICAL: Components MUST be built with musl target for Alpine!`)
}

func loadConfig() BuildConfig {
	config := defaultConfig

	if root := os.Getenv("ROCK_SOURCE_ROOT"); root != "" {
		config.SourceRoot = root
	}

	if out := os.Getenv("ROCK_OUTPUT_DIR"); out != "" {
		config.OutputDir = out
	}

	if target := os.Getenv("ROCK_TARGET"); target != "" {
		config.Target = target
	}

	if profile := os.Getenv("ROCK_PROFILE"); profile != "" {
		config.Profile = profile
	}

	if features := os.Getenv("ROCK_FEATURES"); features != "" {
		config.Features = strings.Split(features, ",")
	}

	if os.Getenv("ROCK_VERBOSE") == "1" {
		config.Verbose = true
	}

	return config
}

func buildComponent(name string, config BuildConfig) {
	component, exists := components[name]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: unknown component: %s\n", name)
		os.Exit(1)
	}

	fmt.Printf("Building %s...\n", component.Name)
	fmt.Printf("Target: %s\n", config.Target)
	fmt.Printf("Profile: %s\n", config.Profile)

	startTime := time.Now()
	result := performBuild(component, config)
	result.BuildTime = time.Since(startTime)

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(result)
	} else {
		printBuildResult(result)
	}

	if !result.Success {
		os.Exit(1)
	}
}

func buildAll(config BuildConfig) {
	results := []BuildResult{}
	failed := 0

	fmt.Println("Building all ROCK-OS components...")
	fmt.Printf("Target: %s\n", config.Target)
	fmt.Printf("Profile: %s\n", config.Profile)
	fmt.Println("=" + strings.Repeat("=", 60))

	for _, name := range []string{"init", "manager", "agent"} {
		component := components[name]
		fmt.Printf("\nBuilding %s...\n", component.Name)

		startTime := time.Now()
		result := performBuild(component, config)
		result.BuildTime = time.Since(startTime)

		results = append(results, result)
		printBuildResult(result)

		if !result.Success {
			failed++
		}
	}

	fmt.Println("\n" + "=" + strings.Repeat("=", 60))
	fmt.Printf("Build Summary: %d successful, %d failed\n",
		len(results)-failed, failed)

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(results)
	}

	if failed > 0 {
		os.Exit(1)
	}
}

func performBuild(component Component, config BuildConfig) BuildResult {
	result := BuildResult{
		Component: component.Name,
		Target:    config.Target,
	}

	// Determine source path
	sourcePath := filepath.Join(config.SourceRoot, component.SourcePath)

	// Check if source exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		result.Error = fmt.Sprintf("source not found: %s", sourcePath)
		return result
	}

	// Check for Cargo.toml
	cargoToml := filepath.Join(sourcePath, "Cargo.toml")
	if _, err := os.Stat(cargoToml); os.IsNotExist(err) {
		result.Error = fmt.Sprintf("Cargo.toml not found in %s", sourcePath)
		return result
	}

	// Build cargo command
	args := []string{"build"}

	// Add target
	args = append(args, "--target", config.Target)

	// Add profile
	if config.Profile == "release" {
		args = append(args, "--release")
	}

	// Add features
	if len(config.Features) > 0 {
		args = append(args, "--features", strings.Join(config.Features, ","))
	}

	// Execute cargo build
	cmd := exec.Command("cargo", args...)
	cmd.Dir = sourcePath

	// Set environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "RUSTFLAGS=-C target-feature=+crt-static")

	if config.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			result.Error = fmt.Sprintf("build failed: %v", err)
			return result
		}
	} else {
		// Capture output for error reporting
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Error = fmt.Sprintf("build failed: %v\n%s", err, output)
			return result
		}
	}

	// Determine output binary path
	targetDir := filepath.Join(sourcePath, "target", config.Target)
	if config.Profile == "release" {
		targetDir = filepath.Join(targetDir, "release")
	} else {
		targetDir = filepath.Join(targetDir, "debug")
	}

	binaryPath := filepath.Join(targetDir, component.BinaryName)

	// Check if binary was created
	stat, err := os.Stat(binaryPath)
	if err != nil {
		result.Error = fmt.Sprintf("binary not found after build: %s", binaryPath)
		return result
	}

	// Create output directory
	outputPath := filepath.Join(config.OutputDir, component.OutputPath)
	outputDir := filepath.Dir(outputPath)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		result.Error = fmt.Sprintf("failed to create output directory: %v", err)
		return result
	}

	// Copy binary to output
	if err := copyBinary(binaryPath, outputPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy binary: %v", err)
		return result
	}

	// Verify it's statically linked (for musl)
	if strings.Contains(config.Target, "musl") {
		result.IsStaticBin = verifyStaticBinary(outputPath)
	}

	result.Success = true
	result.OutputPath = outputPath
	result.Size = stat.Size()

	return result
}

func checkBuildEnvironment() {
	fmt.Println("Checking build environment...")
	fmt.Println("=" + strings.Repeat("=", 60))

	checks := []struct {
		name    string
		command string
		args    []string
	}{
		{"Rust", "rustc", []string{"--version"}},
		{"Cargo", "cargo", []string{"--version"}},
		{"Target", "rustup", []string{"target", "list", "--installed"}},
	}

	allGood := true
	for _, check := range checks {
		cmd := exec.Command(check.command, check.args...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			fmt.Printf("❌ %s: not found\n", check.name)
			allGood = false
			continue
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		fmt.Printf("✅ %s: %s\n", check.name, lines[0])

		// For target list, check for musl
		if check.name == "Target" {
			hasMusl := false
			for _, line := range lines {
				if strings.Contains(line, "musl") {
					hasMusl = true
					fmt.Printf("   ✅ musl target: %s\n", line)
				}
			}
			if !hasMusl {
				fmt.Println("   ⚠️  No musl targets installed")
				fmt.Println("   Run: rock-build setup")
			}
		}
	}

	fmt.Println("=" + strings.Repeat("=", 60))

	if !allGood {
		fmt.Println("\n⚠️  Build environment is incomplete!")
		fmt.Println("\nTo fix:")
		fmt.Println("  1. Install Rust: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh")
		fmt.Println("  2. Install musl target: rustup target add x86_64-unknown-linux-musl")
		os.Exit(1)
	} else {
		fmt.Println("\n✅ Build environment is ready!")
	}
}

func setupRustTarget(target string) {
	fmt.Printf("Setting up Rust target: %s\n", target)

	// Check if rustup is available
	if _, err := exec.LookPath("rustup"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: rustup not found\n")
		fmt.Println("\nInstall Rust:")
		fmt.Println("  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh")
		os.Exit(1)
	}

	// Add target
	cmd := exec.Command("rustup", "target", "add", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add target: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Target %s is ready\n", target)

	// Additional setup for cross-compilation on macOS
	if runtime.GOOS == "darwin" {
		fmt.Println("\n📝 Note for macOS users:")
		fmt.Println("For cross-compilation to Linux, you may need:")
		fmt.Println("  brew install FiloSottile/musl-cross/musl-cross")
		fmt.Println("  brew install cmake")
		fmt.Println("\nSet in your shell:")
		fmt.Println("  export CC_x86_64_unknown_linux_musl=x86_64-linux-musl-gcc")
		fmt.Println("  export CXX_x86_64_unknown_linux_musl=x86_64-linux-musl-g++")
		fmt.Println("  export AR_x86_64_unknown_linux_musl=x86_64-linux-musl-ar")
		fmt.Println("\nFor Alpine-specific builds:")
		fmt.Println("  rustup target add x86_64-alpine-linux-musl")
	}
}

func cleanBuildArtifacts(config BuildConfig) {
	fmt.Println("Cleaning build artifacts...")

	// Clean output directory
	if err := os.RemoveAll(config.OutputDir); err != nil {
		fmt.Printf("⚠️  Failed to clean output: %v\n", err)
	} else {
		fmt.Printf("✅ Cleaned: %s\n", config.OutputDir)
	}

	// Clean each component's target directory
	for name, component := range components {
		sourcePath := filepath.Join(config.SourceRoot, component.SourcePath)
		targetPath := filepath.Join(sourcePath, "target")

		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("Cleaning %s...\n", name)
			cmd := exec.Command("cargo", "clean")
			cmd.Dir = sourcePath

			if err := cmd.Run(); err != nil {
				fmt.Printf("⚠️  Failed to clean %s: %v\n", name, err)
			} else {
				fmt.Printf("✅ Cleaned: %s\n", name)
			}
		}
	}
}

func listComponents() {
	fmt.Println("ROCK-OS Components:")
	fmt.Println("=" + strings.Repeat("=", 60))

	for key, comp := range components {
		fmt.Printf("\n%s:\n", key)
		fmt.Printf("  Name:       %s\n", comp.Name)
		fmt.Printf("  Repository: %s\n", comp.Repository)
		fmt.Printf("  Binary:     %s\n", comp.BinaryName)
		fmt.Printf("  Output:     %s\n", comp.OutputPath)
		fmt.Printf("  Language:   %s\n", comp.Language)
		fmt.Printf("  Target:     %s\n", comp.Target)

		// Check if source exists
		sourcePath := filepath.Join(defaultConfig.SourceRoot, comp.SourcePath)
		if _, err := os.Stat(sourcePath); err == nil {
			fmt.Printf("  Status:     ✅ Source found\n")
		} else {
			fmt.Printf("  Status:     ❌ Source not found\n")
		}
	}

	fmt.Println("\n" + "=" + strings.Repeat("=", 60))
	fmt.Println("\nCRITICAL Integration Paths:")
	fmt.Println("  rock-init → /sbin/init (MUST BE RENAMED!)")
	fmt.Println("  rock-manager → /usr/bin/rock-manager")
	fmt.Println("  volcano-agent → /usr/bin/volcano-agent")
}

func copyBinary(src, dst string) error {
	// Open source
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Get source info
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	// Create destination
	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy contents
	buf := make([]byte, 1024*1024) // 1MB buffer
	for {
		n, err := sourceFile.Read(buf)
		if n > 0 {
			if _, err := destFile.Write(buf[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
	}

	// Preserve permissions
	return os.Chmod(dst, sourceInfo.Mode())
}

func verifyStaticBinary(path string) bool {
	// Use file command to check if statically linked
	cmd := exec.Command("file", path)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	outputStr := string(output)
	return strings.Contains(outputStr, "statically linked") ||
	       strings.Contains(outputStr, "static")
}

func printBuildResult(result BuildResult) {
	if result.Success {
		fmt.Printf("✅ Success: %s\n", result.Component)
		fmt.Printf("   Output: %s\n", result.OutputPath)
		fmt.Printf("   Size: %s\n", formatSize(result.Size))
		fmt.Printf("   Time: %.2fs\n", result.BuildTime.Seconds())
		if result.IsStaticBin {
			fmt.Printf("   Type: Static binary (perfect for Alpine!)\n")
		}
	} else {
		fmt.Printf("❌ Failed: %s\n", result.Component)
		fmt.Printf("   Error: %s\n", result.Error)
	}
}

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

func outputJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(data)
}

// Helper function to run command and get output
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// Helper function to check if command exists
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}