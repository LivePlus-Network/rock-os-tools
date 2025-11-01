// rock-verify - Integration Verification Tool for ROCK-OS
//
// This tool verifies that initramfs images and rootfs directories
// meet the rock-init integration requirements. It encapsulates
// the logic from verify-rock-init-integration.sh
//
// Usage:
//   rock-verify image <image.cpio.gz>   - Verify an initramfs image
//   rock-verify rootfs <path>            - Verify a rootfs directory
//   rock-verify contract                 - Show the integration contract
//   rock-verify cmdline <cmdline>        - Validate kernel command line
//
// Build:
//   go build -o rock-verify cmd/rock-verify/main.go
//
// This tool is CRITICAL for ensuring rock-init will boot successfully

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rock-os/tools/pkg/integration"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func cmdVerifyImage(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rock-verify image <image.cpio.gz>")
	}

	imagePath := args[0]
	fmt.Printf("Verifying image: %s\n", imagePath)
	fmt.Println("=" + strings.Repeat("=", 50))

	result, err := integration.VerifyImage(imagePath)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	// Output result
	if os.Getenv("ROCK_OUTPUT") == "json" {
		data, _ := json.Marshal(result)
		fmt.Println(string(data))
	} else {
		integration.PrintVerificationResult(result)
	}

	if !result.Success {
		return fmt.Errorf("integration verification failed")
	}

	return nil
}

func cmdVerifyRootfs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rock-verify rootfs <path>")
	}

	rootfsPath := args[0]
	fmt.Printf("Verifying rootfs: %s\n", rootfsPath)
	fmt.Println("=" + strings.Repeat("=", 50))

	result, err := integration.VerifyRootfs(rootfsPath)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	// Output result
	if os.Getenv("ROCK_OUTPUT") == "json" {
		data, _ := json.Marshal(result)
		fmt.Println(string(data))
	} else {
		integration.PrintVerificationResult(result)
	}

	if !result.Success {
		return fmt.Errorf("integration verification failed")
	}

	return nil
}

func cmdShowContract(args []string) error {
	contract := integration.GetContract()

	if os.Getenv("ROCK_OUTPUT") == "json" {
		data, _ := json.MarshalIndent(contract, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println("ROCK-OS Integration Contract")
		fmt.Println("============================")
		fmt.Printf("Version: %s\n\n", contract.Version)

		fmt.Println("Required Binary Mappings:")
		fmt.Println("-------------------------")
		for _, binary := range contract.Binaries {
			fmt.Printf("  %s → %s (mode: %o)\n",
				binary.Source, binary.Destination, binary.Permissions)
		}
		fmt.Println()

		fmt.Println("Required Directories:")
		fmt.Println("--------------------")
		for _, dir := range contract.Directories {
			fmt.Printf("  %s\n", dir)
		}
		fmt.Println()

		fmt.Println("Kernel Parameters:")
		fmt.Println("-----------------")
		fmt.Printf("  Init Path: %s\n", contract.KernelParams.InitPath)
		fmt.Printf("  Required Flags: %v\n", contract.KernelParams.RequiredFlags)
		fmt.Printf("  Debug Flags: %v\n", contract.KernelParams.DebugFlags)
		fmt.Printf("  Production Flags: %v\n", contract.KernelParams.ProductionFlags)
		fmt.Println()

		fmt.Println("Device Nodes:")
		fmt.Println("------------")
		for _, node := range contract.DeviceNodes {
			fmt.Printf("  %s (mode: %o, major: %d, minor: %d)\n",
				node.Path, node.Mode, node.Major, node.Minor)
		}
	}

	return nil
}

func cmdValidateCmdline(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rock-verify cmdline <cmdline>")
	}

	cmdline := strings.Join(args, " ")
	fmt.Printf("Validating kernel command line:\n  %s\n\n", cmdline)

	if err := integration.ValidateKernelCmdline(cmdline); err != nil {
		fmt.Printf("❌ INVALID: %v\n", err)
		return err
	}

	fmt.Println("✅ VALID: Kernel command line is correct")

	// Show what the correct cmdline should look like
	fmt.Println("\nExample correct command lines:")
	fmt.Printf("  Debug:      %s\n", integration.GetKernelCmdline("debug"))
	fmt.Printf("  Production: %s\n", integration.GetKernelCmdline("production"))

	return nil
}

func cmdIntegration(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rock-verify integration <image.cpio.gz>")
	}

	// This is the main integration verification command
	// It performs a complete verification of an image
	imagePath := args[0]

	fmt.Println("ROCK-INIT INTEGRATION VERIFICATION")
	fmt.Println("===================================")
	fmt.Printf("Image: %s\n", imagePath)
	fmt.Println()

	// Verify the image
	result, err := integration.VerifyImage(imagePath)
	if err != nil {
		return fmt.Errorf("verification error: %w", err)
	}

	// Print detailed results
	integration.PrintVerificationResult(result)

	// Show contract for reference
	if !result.Success {
		fmt.Println("\nREFERENCE - Required Structure:")
		fmt.Println("--------------------------------")
		contract := integration.GetContract()
		for _, binary := range contract.Binaries {
			fmt.Printf("  %s → %s\n", binary.Source, binary.Destination)
		}
		fmt.Println("\nNOTE: These paths are hardcoded in rock-init and cannot be changed!")
	}

	if !result.Success {
		return fmt.Errorf("integration verification failed - rock-init will not boot")
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("rock-verify - Integration Verification Tool for ROCK-OS")
		fmt.Println()
		fmt.Println("This tool verifies that images and rootfs directories meet")
		fmt.Println("the rock-init integration requirements. Use it to ensure")
		fmt.Println("your images will boot successfully.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  rock-verify integration <image>  Complete integration check (recommended)")
		fmt.Println("  rock-verify image <image>        Verify an initramfs image")
		fmt.Println("  rock-verify rootfs <path>        Verify a rootfs directory")
		fmt.Println("  rock-verify contract             Show the integration contract")
		fmt.Println("  rock-verify cmdline <cmdline>    Validate kernel command line")
		fmt.Println("  rock-verify version              Show version information")
		fmt.Println()
		fmt.Println("Environment:")
		fmt.Println("  ROCK_OUTPUT=json                 Output JSON for scripting")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  rock-verify integration initrd.cpio.gz")
		fmt.Println("  rock-verify rootfs ./build/rootfs")
		fmt.Println("  rock-verify cmdline \"init=/sbin/init console=ttyS0\"")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Handle version command
	if command == "version" {
		fmt.Printf("rock-verify version %s (built %s, commit %s)\n", Version, BuildTime, GitCommit)
		return
	}

	var err error
	switch command {
	case "integration":
		err = cmdIntegration(args)
	case "image":
		err = cmdVerifyImage(args)
	case "rootfs":
		err = cmdVerifyRootfs(args)
	case "contract":
		err = cmdShowContract(args)
	case "cmdline":
		err = cmdValidateCmdline(args)
	default:
		err = fmt.Errorf("unknown command: %s", command)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}