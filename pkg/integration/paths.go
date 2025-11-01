// Package integration defines the critical integration contract with rock-init
// THESE PATHS ARE HARDCODED IN ROCK-INIT AND MUST NOT BE CHANGED
package integration

// Critical paths hardcoded in rock-init
// Changing these will cause ROCK-OS to fail to boot
const (
	// RockInitPath is where rock-init MUST be renamed to
	// Referenced throughout rock-init code
	RockInitPath = "/sbin/init"

	// RockManagerPath is hardcoded at line 553 in rock-init
	RockManagerPath = "/usr/bin/rock-manager"

	// VolcanoAgentPath is hardcoded at lines 331 and 581 in rock-init
	VolcanoAgentPath = "/usr/bin/volcano-agent"

	// ConfigKeyPath is hardcoded at line 438 in rock-init
	ConfigKeyPath = "/config/CONFIG_KEY"

	// BusyboxPath is the standard location for busybox
	BusyboxPath = "/bin/busybox"

	// ShellPath must be a symlink to busybox
	ShellPath = "/bin/sh"

	// KernelCmdlineInit is the correct kernel parameter
	// Must use "init=" NOT "rdinit="
	KernelCmdlineInit = "init=/sbin/init"
)

// RequiredDirectories are the directories that must exist in the initramfs
var RequiredDirectories = []string{
	"/proc",
	"/sys",
	"/dev",
	"/tmp",
	"/run",
	"/var/log",
	"/sbin",
	"/bin",
	"/usr/bin",
	"/config",
	"/etc/rock",
}

// BinaryMapping defines how binaries should be placed
type BinaryMapping struct {
	Source      string // Source binary name
	Destination string // Destination path in initramfs
	Permissions uint32 // Unix permissions
}

// RequiredBinaries defines the mandatory binary mappings
var RequiredBinaries = []BinaryMapping{
	{
		Source:      "rock-init",
		Destination: RockInitPath,
		Permissions: 0755,
	},
	{
		Source:      "rock-manager",
		Destination: RockManagerPath,
		Permissions: 0755,
	},
	{
		Source:      "volcano-agent",
		Destination: VolcanoAgentPath,
		Permissions: 0755,
	},
	{
		Source:      "busybox",
		Destination: BusyboxPath,
		Permissions: 0755,
	},
}

// BusyboxSymlinks are the required symlinks to busybox
var BusyboxSymlinks = []string{
	"sh",
	"ls",
	"cat",
	"echo",
	"mount",
	"umount",
	"mkdir",
	"rm",
	"cp",
	"mv",
	"chmod",
	"chown",
	"sleep",
	"test",
	"[",
	"[[",
}

// DeviceNodes defines required device nodes
type DeviceNode struct {
	Path  string
	Mode  uint32
	Major uint32
	Minor uint32
}

// RequiredDeviceNodes are the device nodes that must be created
var RequiredDeviceNodes = []DeviceNode{
	{Path: "/dev/null", Mode: 0666, Major: 1, Minor: 3},
	{Path: "/dev/zero", Mode: 0666, Major: 1, Minor: 5},
	{Path: "/dev/random", Mode: 0666, Major: 1, Minor: 8},
	{Path: "/dev/urandom", Mode: 0666, Major: 1, Minor: 9},
	{Path: "/dev/tty", Mode: 0666, Major: 5, Minor: 0},
	{Path: "/dev/console", Mode: 0620, Major: 5, Minor: 1},
	{Path: "/dev/ptmx", Mode: 0666, Major: 5, Minor: 2},
}