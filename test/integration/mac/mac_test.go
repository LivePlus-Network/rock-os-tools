package mac_test

import (
    "os"
    "os/exec"
    "testing"
)

func TestRockMacBuild(t *testing.T) {
    // Test that rock-mac builds successfully
    cmd := exec.Command("go", "build", "-o", "/tmp/rock-mac-test", "../../../cmd/rock-mac")
    if output, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("Failed to build rock-mac: %v\nOutput: %s", err, output)
    }
    defer os.Remove("/tmp/rock-mac-test")

    // Test that it runs
    cmd = exec.Command("/tmp/rock-mac-test", "--version")
    if output, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("Failed to run rock-mac: %v\nOutput: %s", err, output)
    }
}

func TestDatabaseExists(t *testing.T) {
    // Check that database exists
    dbPath := os.Getenv("HOME") + "/.rock/mac-dispenser.db"
    if _, err := os.Stat(dbPath); os.IsNotExist(err) {
        t.Skip("Database not initialized, skipping test")
    }
}
