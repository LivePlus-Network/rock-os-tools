package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"
	cfgFile string
	verbose bool
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "rock-mac",
		Short: "ROCK OS MAC Address Dispenser",
		Long: `rock-mac manages MAC address allocation for ROCK OS nodes.

All MAC addresses use the ROCK OS OUI prefix: a4:58:0f
Addresses are organized into pools for different environments.`,
		Version: version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.rock/mac-dispenser.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Add commands
	rootCmd.AddCommand(
		newAllocateCmd(),
		newListCmd(),
		newReleaseCmd(),
		newReserveCmd(),
		newStatsCmd(),
		newShowCmd(),
		newInitCmd(),
		newCleanupCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// newAllocateCmd creates the allocate command
func newAllocateCmd() *cobra.Command {
	var (
		pool       string
		deviceID   string
		deviceType string
		metadata   string
	)

	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Allocate a new MAC address",
		Long:  `Allocate a new MAC address from the specified pool.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			mac, err := AllocateMAC(db, pool, deviceID, deviceType, metadata)
			if err != nil {
				return fmt.Errorf("failed to allocate MAC: %w", err)
			}

			fmt.Printf("Allocated MAC: %s\n", mac)
			if verbose {
				fmt.Printf("Pool: %s\n", pool)
				fmt.Printf("Device ID: %s\n", deviceID)
				fmt.Printf("Device Type: %s\n", deviceType)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&pool, "pool", "p", "development", "Pool to allocate from")
	cmd.Flags().StringVarP(&deviceID, "device", "d", "", "Device ID")
	cmd.Flags().StringVarP(&deviceType, "type", "t", "qemu-vm", "Device type")
	cmd.Flags().StringVarP(&metadata, "metadata", "m", "{}", "JSON metadata")

	return cmd
}

// newListCmd creates the list command
func newListCmd() *cobra.Command {
	var (
		pool   string
		status string
		limit  int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List allocated MAC addresses",
		Long:  `List MAC addresses with optional filters.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			allocations, err := ListAllocations(db, pool, status, limit)
			if err != nil {
				return fmt.Errorf("failed to list allocations: %w", err)
			}

			if len(allocations) == 0 {
				fmt.Println("No allocations found")
				return nil
			}

			// Print header
			fmt.Printf("%-20s %-12s %-20s %-10s %s\n",
				"MAC Address", "Pool", "Device ID", "Status", "Allocated At")
			fmt.Println(String(80, "-"))

			// Print allocations
			for _, a := range allocations {
				fmt.Printf("%-20s %-12s %-20s %-10s %s\n",
					a.MACAddress, a.Pool, a.DeviceID, a.Status, a.AllocatedAt.Format("2006-01-02 15:04"))
			}

			fmt.Printf("\nTotal: %d allocation(s)\n", len(allocations))
			return nil
		},
	}

	cmd.Flags().StringVarP(&pool, "pool", "p", "", "Filter by pool")
	cmd.Flags().StringVarP(&status, "status", "s", "active", "Filter by status (active, released, reserved)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "Limit results")

	return cmd
}

// newReleaseCmd creates the release command
func newReleaseCmd() *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "release <mac-address|device-id>",
		Short: "Release a MAC address",
		Long:  `Release a MAC address back to the pool.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			identifier := args[0]
			count, err := ReleaseMAC(db, identifier, force)
			if err != nil {
				return fmt.Errorf("failed to release MAC: %w", err)
			}

			if count == 0 {
				fmt.Printf("No active allocations found for: %s\n", identifier)
			} else {
				fmt.Printf("Released %d MAC address(es)\n", count)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force release even if reserved")

	return cmd
}

// newReserveCmd creates the reserve command
func newReserveCmd() *cobra.Command {
	var (
		pool       string
		deviceID   string
		deviceType string
		metadata   string
		specific   string
	)

	cmd := &cobra.Command{
		Use:   "reserve",
		Short: "Reserve a specific MAC address",
		Long:  `Reserve a specific MAC address or the next available in a pool.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			var mac string
			if specific != "" {
				mac, err = ReserveSpecificMAC(db, specific, pool, deviceID, deviceType, metadata)
			} else {
				mac, err = ReserveNextMAC(db, pool, deviceID, deviceType, metadata)
			}

			if err != nil {
				return fmt.Errorf("failed to reserve MAC: %w", err)
			}

			fmt.Printf("Reserved MAC: %s\n", mac)
			if verbose {
				fmt.Printf("Pool: %s\n", pool)
				fmt.Printf("Device ID: %s\n", deviceID)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&pool, "pool", "p", "reserved", "Pool for reservation")
	cmd.Flags().StringVarP(&specific, "mac", "m", "", "Specific MAC to reserve (e.g., a4:58:0f:00:00:01)")
	cmd.Flags().StringVarP(&deviceID, "device", "d", "", "Device ID")
	cmd.Flags().StringVarP(&deviceType, "type", "t", "reserved", "Device type")
	cmd.Flags().StringVar(&metadata, "metadata", "{}", "JSON metadata")

	return cmd
}

// newStatsCmd creates the stats command
func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show pool statistics",
		Long:  `Display statistics for all MAC address pools.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			stats, err := GetPoolStats(db)
			if err != nil {
				return fmt.Errorf("failed to get stats: %w", err)
			}

			// Print header
			fmt.Printf("%-12s %-30s %8s %10s %10s %10s\n",
				"Pool", "Description", "Active", "Released", "Reserved", "Total")
			fmt.Println(String(90, "-"))

			// Print stats
			var totalActive, totalReleased, totalReserved, totalAll int
			for _, s := range stats {
				fmt.Printf("%-12s %-30s %8d %10d %10d %10d\n",
					s.Pool, s.Description, s.ActiveCount, s.ReleasedCount,
					s.ReservedCount, s.TotalAllocated)

				totalActive += s.ActiveCount
				totalReleased += s.ReleasedCount
				totalReserved += s.ReservedCount
				totalAll += s.TotalAllocated
			}

			// Print totals
			fmt.Println(String(90, "-"))
			fmt.Printf("%-12s %-30s %8d %10d %10d %10d\n",
				"TOTAL", "", totalActive, totalReleased, totalReserved, totalAll)

			return nil
		},
	}

	return cmd
}

// newShowCmd creates the show command
func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <mac-address>",
		Short: "Show details for a specific MAC address",
		Long:  `Display detailed information about a specific MAC address.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			mac := args[0]
			allocation, err := GetAllocation(db, mac)
			if err != nil {
				return fmt.Errorf("failed to get allocation: %w", err)
			}

			if allocation == nil {
				fmt.Printf("MAC address not found: %s\n", mac)
				return nil
			}

			// Print details
			fmt.Printf("MAC Address:  %s\n", allocation.MACAddress)
			fmt.Printf("Pool:         %s\n", allocation.Pool)
			fmt.Printf("Status:       %s\n", allocation.Status)
			fmt.Printf("Device ID:    %s\n", allocation.DeviceID)
			fmt.Printf("Device Type:  %s\n", allocation.DeviceType)
			fmt.Printf("Allocated At: %s\n", allocation.AllocatedAt.Format("2006-01-02 15:04:05"))

			if allocation.ReleasedAt != nil {
				fmt.Printf("Released At:  %s\n", allocation.ReleasedAt.Format("2006-01-02 15:04:05"))
			}
			if allocation.LastSeen != nil {
				fmt.Printf("Last Seen:    %s\n", allocation.LastSeen.Format("2006-01-02 15:04:05"))
			}
			if allocation.Metadata != "" && allocation.Metadata != "{}" {
				fmt.Printf("Metadata:     %s\n", allocation.Metadata)
			}

			return nil
		},
	}

	return cmd
}

// newInitCmd creates the init command
func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the MAC dispenser database",
		Long:  `Initialize or reinitialize the MAC dispenser database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Run the initialization script
			initScript := "/Volumes/4TB/rock-os-tools/scripts/init-mac-dispenser.sh"
			if _, err := os.Stat(initScript); err != nil {
				return fmt.Errorf("initialization script not found: %s", initScript)
			}

			fmt.Println("Initializing MAC dispenser database...")
			if err := RunCommand(initScript); err != nil {
				return fmt.Errorf("initialization failed: %w", err)
			}

			fmt.Println("Database initialized successfully!")
			return nil
		},
	}

	return cmd
}

// newCleanupCmd creates the cleanup command
func newCleanupCmd() *cobra.Command {
	var (
		dryRun bool
		days   int
	)

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up expired allocations",
		Long:  `Release MAC addresses that have exceeded their auto-release period.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			count, err := CleanupExpired(db, days, dryRun)
			if err != nil {
				return fmt.Errorf("cleanup failed: %w", err)
			}

			if dryRun {
				fmt.Printf("Would release %d expired allocation(s)\n", count)
			} else {
				fmt.Printf("Released %d expired allocation(s)\n", count)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be released without making changes")
	cmd.Flags().IntVarP(&days, "days", "d", 0, "Override auto-release days (0 = use pool defaults)")

	return cmd
}

// String creates a string of repeated characters
func String(n int, char string) string {
	result := ""
	for i := 0; i < n; i++ {
		result += char
	}
	return result
}