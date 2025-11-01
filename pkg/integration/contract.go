package integration

import (
	"fmt"
	"strings"
)

// IntegrationContract defines the complete contract between rock-os-tools and rock-init
// This is the source of truth for all integration requirements
type IntegrationContract struct {
	Version      string
	Binaries     []BinaryMapping
	Directories  []string
	DeviceNodes  []DeviceNode
	KernelParams KernelParameters
}

// KernelParameters defines required kernel command line parameters
type KernelParameters struct {
	InitPath      string   // Must be "init=/sbin/init" NOT "rdinit="
	RequiredFlags []string // Additional required flags
	DebugFlags    []string // Flags for debug mode
	ProductionFlags []string // Flags for production mode
}

// GetContract returns the current integration contract
func GetContract() *IntegrationContract {
	return &IntegrationContract{
		Version:     "1.0",
		Binaries:    RequiredBinaries,
		Directories: RequiredDirectories,
		DeviceNodes: RequiredDeviceNodes,
		KernelParams: KernelParameters{
			InitPath: KernelCmdlineInit,
			RequiredFlags: []string{
				"net.ifnames=0", // Predictable network interface names
			},
			DebugFlags: []string{
				"console=ttyS0",
				"debug",
			},
			ProductionFlags: []string{
				"quiet",
				"security=selinux",
			},
		},
	}
}

// GetKernelCmdline returns the correct kernel command line for a given mode
func GetKernelCmdline(mode string) string {
	contract := GetContract()

	// CRITICAL: Always start with the correct init path
	cmdline := contract.KernelParams.InitPath

	// Add required flags
	for _, flag := range contract.KernelParams.RequiredFlags {
		cmdline += " " + flag
	}

	// Add mode-specific flags
	switch mode {
	case "debug":
		for _, flag := range contract.KernelParams.DebugFlags {
			cmdline += " " + flag
		}
	case "production":
		for _, flag := range contract.KernelParams.ProductionFlags {
			cmdline += " " + flag
		}
	default:
		// Default to debug mode for safety
		for _, flag := range contract.KernelParams.DebugFlags {
			cmdline += " " + flag
		}
	}

	return cmdline
}

// ValidateKernelCmdline checks if a kernel command line is correct
func ValidateKernelCmdline(cmdline string) error {
	// Check for the critical init parameter
	if !strings.Contains(cmdline, "init=/sbin/init") {
		return fmt.Errorf("kernel cmdline missing required 'init=/sbin/init' parameter")
	}

	// Check for incorrect rdinit parameter
	if strings.Contains(cmdline, "rdinit=") {
		return fmt.Errorf("kernel cmdline uses 'rdinit=' which is incorrect; must use 'init='")
	}

	return nil
}