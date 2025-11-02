package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/rock-os-tools/pkg/mac"
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

// Command implementations
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
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			macAddr, err := mac.AllocateMAC(db, pool, deviceID, deviceType, metadata)
			if err != nil {
				return fmt.Errorf("failed to allocate MAC: %w", err)
			}

			fmt.Printf("Allocated MAC: %s\n", macAddr)
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
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			allocations, err := mac.ListAllocations(db, pool, status, limit)
			if err != nil {
				return fmt.Errorf("failed to list allocations: %w", err)
			}

			if len(allocations) == 0 {
				fmt.Println("No allocations found")
				return nil
			}

			fmt.Printf("%-20s %-12s %-20s %-10s %s\n",
				"MAC Address", "Pool", "Device ID", "Status", "Allocated At")
			fmt.Println(mac.String(80, "-"))

			for _, a := range allocations {
				fmt.Printf("%-20s %-12s %-20s %-10s %s\n",
					a.MACAddress, a.Pool, a.DeviceID, a.Status, a.AllocatedAt.Format("2006-01-02 15:04"))
			}

			fmt.Printf("\nTotal: %d allocation(s)\n", len(allocations))
			return nil
		},
	}

	cmd.Flags().StringVarP(&pool, "pool", "p", "", "Filter by pool")
	cmd.Flags().StringVarP(&status, "status", "s", "active", "Filter by status")
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "Limit results")

	return cmd
}

func newReleaseCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "release <mac-address|device-id>",
		Short: "Release a MAC address",
		Long:  `Release a MAC address back to the pool.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			count, err := mac.ReleaseMAC(db, args[0], force)
			if err != nil {
				return fmt.Errorf("failed to release MAC: %w", err)
			}

			if count == 0 {
				fmt.Printf("No active allocations found for: %s\n", args[0])
			} else {
				fmt.Printf("Released %d MAC address(es)\n", count)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force release")
	return cmd
}

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
		Short: "Reserve a MAC address",
		Long:  `Reserve a specific MAC address or the next available.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			var macAddr string
			if specific != "" {
				macAddr, err = mac.ReserveSpecificMAC(db, specific, pool, deviceID, deviceType, metadata)
			} else {
				macAddr, err = mac.ReserveNextMAC(db, pool, deviceID, deviceType, metadata)
			}

			if err != nil {
				return fmt.Errorf("failed to reserve MAC: %w", err)
			}

			fmt.Printf("Reserved MAC: %s\n", macAddr)
			return nil
		},
	}

	cmd.Flags().StringVarP(&pool, "pool", "p", "reserved", "Pool")
	cmd.Flags().StringVarP(&specific, "mac", "m", "", "Specific MAC")
	cmd.Flags().StringVarP(&deviceID, "device", "d", "", "Device ID")
	cmd.Flags().StringVarP(&deviceType, "type", "t", "reserved", "Device type")
	cmd.Flags().StringVar(&metadata, "metadata", "{}", "JSON metadata")

	return cmd
}

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show pool statistics",
		Long:  `Display statistics for all MAC address pools.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			stats, err := mac.GetPoolStats(db)
			if err != nil {
				return fmt.Errorf("failed to get stats: %w", err)
			}

			fmt.Printf("%-12s %-30s %8s %10s %10s\n",
				"Pool", "Description", "Active", "Released", "Reserved")
			fmt.Println(mac.String(80, "-"))

			for _, s := range stats {
				fmt.Printf("%-12s %-30s %8d %10d %10d\n",
					s.Pool, s.Description, s.ActiveCount, s.ReleasedCount, s.ReservedCount)
			}

			return nil
		},
	}

	return cmd
}

func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <mac-address>",
		Short: "Show MAC details",
		Long:  `Display detailed information about a MAC address.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			allocation, err := mac.GetAllocation(db, args[0])
			if err != nil {
				return fmt.Errorf("failed to get allocation: %w", err)
			}

			if allocation == nil {
				fmt.Printf("MAC address not found: %s\n", args[0])
				return nil
			}

			fmt.Printf("MAC Address:  %s\n", allocation.MACAddress)
			fmt.Printf("Pool:         %s\n", allocation.Pool)
			fmt.Printf("Status:       %s\n", allocation.Status)
			fmt.Printf("Device ID:    %s\n", allocation.DeviceID)
			fmt.Printf("Allocated At: %s\n", allocation.AllocatedAt.Format("2006-01-02 15:04:05"))

			return nil
		},
	}

	return cmd
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize database",
		Long:  `Initialize the MAC dispenser database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			initScript := "/Volumes/4TB/rock-os-tools/scripts/init-mac-dispenser.sh"
			if _, err := os.Stat(initScript); err != nil {
				return fmt.Errorf("init script not found: %s", initScript)
			}

			fmt.Println("Initializing MAC dispenser database...")
			if err := mac.RunCommand(initScript); err != nil {
				return fmt.Errorf("initialization failed: %w", err)
			}

			fmt.Println("Database initialized successfully!")
			return nil
		},
	}

	return cmd
}

func newCleanupCmd() *cobra.Command {
	var (
		dryRun bool
		days   int
	)

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up expired allocations",
		Long:  `Release expired MAC addresses.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mac.OpenDatabase()
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			count, err := mac.CleanupExpired(db, days, dryRun)
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

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be released")
	cmd.Flags().IntVarP(&days, "days", "d", 0, "Override auto-release days")

	return cmd
}
