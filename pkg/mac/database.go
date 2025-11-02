package mac

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// MACPrefix is the ROCK OS OUI
const MACPrefix = "a4:58:0f"

// Allocation represents a MAC address allocation
type Allocation struct {
	ID          int
	MACAddress  string
	Pool        string
	DeviceID    string
	DeviceType  string
	Metadata    string
	AllocatedAt time.Time
	ReleasedAt  *time.Time
	LastSeen    *time.Time
	Status      string
}

// PoolStats represents statistics for a pool
type PoolStats struct {
	Pool           string
	Description    string
	ActiveCount    int
	ReleasedCount  int
	ReservedCount  int
	TotalAllocated int
	TotalReleased  int
}

// Pool represents a MAC address pool configuration
type Pool struct {
	Name            string
	RangeStart      string
	RangeEnd        string
	Description     string
	AutoReleaseDays int
}

// OpenDatabase opens the SQLite database
func OpenDatabase() (*sql.DB, error) {
	dbPath := filepath.Join(os.Getenv("HOME"), ".rock", "mac-dispenser.db")

	// Ensure database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database not found at %s - run 'rock-mac init' first", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// AllocateMAC allocates a new MAC address from the specified pool
func AllocateMAC(db *sql.DB, pool, deviceID, deviceType, metadata string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Get the last allocated address for this pool
	var lastAllocated string
	err = tx.QueryRow(`
		SELECT last_allocated FROM counters WHERE pool = ?
	`, pool).Scan(&lastAllocated)
	if err != nil {
		return "", fmt.Errorf("failed to get counter for pool %s: %w", pool, err)
	}

	// Calculate next MAC address
	nextMAC := incrementMAC(lastAllocated)
	fullMAC := fmt.Sprintf("%s:%s", MACPrefix, nextMAC)

	// Insert allocation
	_, err = tx.Exec(`
		INSERT INTO mac_allocations (mac_address, pool, device_id, device_type, metadata, status)
		VALUES (?, ?, ?, ?, ?, 'active')
	`, fullMAC, pool, deviceID, deviceType, metadata)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("MAC address %s already allocated", fullMAC)
		}
		return "", err
	}

	// Update counter
	_, err = tx.Exec(`
		UPDATE counters
		SET last_allocated = ?, total_allocated = total_allocated + 1, updated_at = CURRENT_TIMESTAMP
		WHERE pool = ?
	`, nextMAC, pool)
	if err != nil {
		return "", err
	}

	// Add audit log entry
	auditData := map[string]string{
		"pool":        pool,
		"device_id":   deviceID,
		"device_type": deviceType,
	}
	auditJSON, _ := json.Marshal(auditData)

	_, err = tx.Exec(`
		INSERT INTO audit_log (action, mac_address, pool, device_id, user, details)
		VALUES ('allocate', ?, ?, ?, ?, ?)
	`, fullMAC, pool, deviceID, os.Getenv("USER"), string(auditJSON))
	if err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}

	return fullMAC, nil
}

// ListAllocations lists MAC allocations with optional filters
func ListAllocations(db *sql.DB, pool, status string, limit int) ([]*Allocation, error) {
	query := `
		SELECT id, mac_address, pool, device_id, device_type, metadata,
		       allocated_at, released_at, last_seen, status
		FROM mac_allocations
		WHERE 1=1
	`
	args := []interface{}{}

	if pool != "" {
		query += " AND pool = ?"
		args = append(args, pool)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY allocated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allocations []*Allocation
	for rows.Next() {
		a := &Allocation{}
		err := rows.Scan(&a.ID, &a.MACAddress, &a.Pool, &a.DeviceID,
			&a.DeviceType, &a.Metadata, &a.AllocatedAt,
			&a.ReleasedAt, &a.LastSeen, &a.Status)
		if err != nil {
			return nil, err
		}
		allocations = append(allocations, a)
	}

	return allocations, nil
}

// ReleaseMAC releases a MAC address back to the pool
func ReleaseMAC(db *sql.DB, identifier string, force bool) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Build query based on identifier type
	query := `
		UPDATE mac_allocations
		SET status = 'released', released_at = CURRENT_TIMESTAMP
		WHERE status = 'active'
	`

	if strings.Contains(identifier, ":") {
		// It's a MAC address
		query += " AND mac_address = ?"
	} else {
		// It's a device ID
		query += " AND device_id = ?"
	}

	if !force {
		query += " AND status != 'reserved'"
	}

	result, err := tx.Exec(query, identifier)
	if err != nil {
		return 0, err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if count > 0 {
		// Update counter
		_, err = tx.Exec(`
			UPDATE counters
			SET total_released = total_released + ?, updated_at = CURRENT_TIMESTAMP
			WHERE pool IN (
				SELECT pool FROM mac_allocations
				WHERE mac_address = ? OR device_id = ?
			)
		`, count, identifier, identifier)
		if err != nil {
			return 0, err
		}

		// Add audit log
		auditData := map[string]interface{}{
			"identifier": identifier,
			"forced":     force,
			"count":      count,
		}
		auditJSON, _ := json.Marshal(auditData)

		_, err = tx.Exec(`
			INSERT INTO audit_log (action, mac_address, pool, device_id, user, details)
			VALUES ('release', ?, '', ?, ?, ?)
		`, identifier, identifier, os.Getenv("USER"), string(auditJSON))
		if err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return count, nil
}

// ReserveSpecificMAC reserves a specific MAC address
func ReserveSpecificMAC(db *sql.DB, mac, pool, deviceID, deviceType, metadata string) (string, error) {
	// Validate MAC format
	if !strings.HasPrefix(mac, MACPrefix) {
		return "", fmt.Errorf("MAC must start with %s", MACPrefix)
	}

	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Check if already allocated
	var existing string
	err = tx.QueryRow("SELECT mac_address FROM mac_allocations WHERE mac_address = ?", mac).Scan(&existing)
	if err == nil {
		return "", fmt.Errorf("MAC %s already allocated", mac)
	} else if err != sql.ErrNoRows {
		return "", err
	}

	// Insert reservation
	_, err = tx.Exec(`
		INSERT INTO mac_allocations (mac_address, pool, device_id, device_type, metadata, status)
		VALUES (?, ?, ?, ?, ?, 'reserved')
	`, mac, pool, deviceID, deviceType, metadata)
	if err != nil {
		return "", err
	}

	// Add audit log
	auditData := map[string]string{
		"pool":        pool,
		"device_id":   deviceID,
		"device_type": deviceType,
		"specific":    "true",
	}
	auditJSON, _ := json.Marshal(auditData)

	_, err = tx.Exec(`
		INSERT INTO audit_log (action, mac_address, pool, device_id, user, details)
		VALUES ('reserve', ?, ?, ?, ?, ?)
	`, mac, pool, deviceID, os.Getenv("USER"), string(auditJSON))
	if err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}

	return mac, nil
}

// ReserveNextMAC reserves the next available MAC in a pool
func ReserveNextMAC(db *sql.DB, pool, deviceID, deviceType, metadata string) (string, error) {
	// Similar to AllocateMAC but with status='reserved'
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var lastAllocated string
	err = tx.QueryRow(`
		SELECT last_allocated FROM counters WHERE pool = ?
	`, pool).Scan(&lastAllocated)
	if err != nil {
		return "", fmt.Errorf("failed to get counter for pool %s: %w", pool, err)
	}

	nextMAC := incrementMAC(lastAllocated)
	fullMAC := fmt.Sprintf("%s:%s", MACPrefix, nextMAC)

	_, err = tx.Exec(`
		INSERT INTO mac_allocations (mac_address, pool, device_id, device_type, metadata, status)
		VALUES (?, ?, ?, ?, ?, 'reserved')
	`, fullMAC, pool, deviceID, deviceType, metadata)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(`
		UPDATE counters
		SET last_allocated = ?, updated_at = CURRENT_TIMESTAMP
		WHERE pool = ?
	`, nextMAC, pool)
	if err != nil {
		return "", err
	}

	auditData := map[string]string{
		"pool":        pool,
		"device_id":   deviceID,
		"device_type": deviceType,
	}
	auditJSON, _ := json.Marshal(auditData)

	_, err = tx.Exec(`
		INSERT INTO audit_log (action, mac_address, pool, device_id, user, details)
		VALUES ('reserve', ?, ?, ?, ?, ?)
	`, fullMAC, pool, deviceID, os.Getenv("USER"), string(auditJSON))
	if err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}

	return fullMAC, nil
}

// GetAllocation gets details for a specific MAC address
func GetAllocation(db *sql.DB, mac string) (*Allocation, error) {
	a := &Allocation{}
	err := db.QueryRow(`
		SELECT id, mac_address, pool, device_id, device_type, metadata,
		       allocated_at, released_at, last_seen, status
		FROM mac_allocations
		WHERE mac_address = ?
	`, mac).Scan(&a.ID, &a.MACAddress, &a.Pool, &a.DeviceID,
		&a.DeviceType, &a.Metadata, &a.AllocatedAt,
		&a.ReleasedAt, &a.LastSeen, &a.Status)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return a, nil
}

// GetPoolStats gets statistics for all pools
func GetPoolStats(db *sql.DB) ([]*PoolStats, error) {
	rows, err := db.Query(`
		SELECT pool, description, active_count, released_count,
		       reserved_count, total_allocated, total_released
		FROM pool_stats
		ORDER BY pool
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*PoolStats
	for rows.Next() {
		s := &PoolStats{}
		err := rows.Scan(&s.Pool, &s.Description, &s.ActiveCount,
			&s.ReleasedCount, &s.ReservedCount, &s.TotalAllocated,
			&s.TotalReleased)
		if err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// CleanupExpired releases expired allocations based on pool auto-release settings
func CleanupExpired(db *sql.DB, overrideDays int, dryRun bool) (int64, error) {
	var count int64

	if dryRun {
		// Count what would be released
		err := db.QueryRow(`
			SELECT COUNT(*) FROM mac_allocations m
			JOIN pools p ON m.pool = p.name
			WHERE m.status = 'active'
			  AND p.auto_release_days > 0
			  AND julianday('now') - julianday(m.allocated_at) > p.auto_release_days
		`).Scan(&count)
		return count, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	query := `
		UPDATE mac_allocations
		SET status = 'released', released_at = CURRENT_TIMESTAMP
		WHERE status = 'active'
		  AND pool IN (
			SELECT name FROM pools
			WHERE auto_release_days > 0
		  )
	`

	if overrideDays > 0 {
		query += fmt.Sprintf(" AND julianday('now') - julianday(allocated_at) > %d", overrideDays)
	} else {
		query += ` AND julianday('now') - julianday(allocated_at) >
			(SELECT auto_release_days FROM pools WHERE pools.name = mac_allocations.pool)`
	}

	result, err := tx.Exec(query)
	if err != nil {
		return 0, err
	}

	count, err = result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if count > 0 {
		// Add audit log
		auditData := map[string]interface{}{
			"action":        "auto_cleanup",
			"count":         count,
			"override_days": overrideDays,
		}
		auditJSON, _ := json.Marshal(auditData)

		_, err = tx.Exec(`
			INSERT INTO audit_log (action, user, details)
			VALUES ('cleanup', ?, ?)
		`, os.Getenv("USER"), string(auditJSON))
		if err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return count, nil
}

// incrementMAC increments a MAC address suffix (last 3 octets)
func incrementMAC(current string) string {
	// Parse current MAC suffix (format: XX:XX:XX)
	parts := strings.Split(current, ":")
	if len(parts) != 3 {
		return "00:00:01" // Default start
	}

	// Convert to single number
	var num int
	fmt.Sscanf(parts[0], "%02x", &num)
	num = num << 16
	var tmp int
	fmt.Sscanf(parts[1], "%02x", &tmp)
	num |= tmp << 8
	fmt.Sscanf(parts[2], "%02x", &tmp)
	num |= tmp

	// Increment
	num++

	// Convert back to MAC format
	return fmt.Sprintf("%02x:%02x:%02x",
		(num>>16)&0xff, (num>>8)&0xff, num&0xff)
}

// RunCommand executes a shell command
func RunCommand(command string) error {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// String creates a string of repeated characters (exported helper function)
func String(n int, char string) string {
	result := ""
	for i := 0; i < n; i++ {
		result += char
	}
	return result
}