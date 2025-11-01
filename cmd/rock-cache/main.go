package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	Version   = "1.0.0"
	BuildTime = "2025-01-01T00:00:00Z"
	GitCommit = "dev"

	// Default cache directory
	DefaultCacheDir = ".rock-cache"

	// Cache subdirectories
	ArtifactsDir = "artifacts"
	MetadataDir  = "metadata"

	// Default cache expiration (7 days)
	DefaultMaxAge = 7 * 24 * time.Hour
)

// CacheEntry represents metadata for a cached artifact
type CacheEntry struct {
	Key         string    `json:"key"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	Hash        string    `json:"hash"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	AccessCount int       `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalEntries int    `json:"total_entries"`
	TotalSize    int64  `json:"total_size"`
	OldestEntry  time.Time `json:"oldest_entry,omitempty"`
	NewestEntry  time.Time `json:"newest_entry,omitempty"`
}

var (
	cacheDir     string
	artifactsDir string
	metadataDir  string
	verboseMode  bool
	jsonOutput   bool
)

func init() {
	// Set up cache directories
	cacheDir = os.Getenv("ROCK_CACHE_DIR")
	if cacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, DefaultCacheDir)
	}

	artifactsDir = filepath.Join(cacheDir, ArtifactsDir)
	metadataDir = filepath.Join(cacheDir, MetadataDir)

	// Check for verbose mode
	if os.Getenv("ROCK_VERBOSE") == "true" {
		verboseMode = true
	}

	// Check for JSON output
	if os.Getenv("ROCK_OUTPUT") == "json" {
		jsonOutput = true
	}
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	// Initialize cache directories
	if err := initializeCacheDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing cache: %v\n", err)
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "store":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: store requires <key> <file> arguments\n")
			showUsage()
			os.Exit(1)
		}
		cmdStore(os.Args[2], os.Args[3])

	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: get requires <key> argument\n")
			showUsage()
			os.Exit(1)
		}
		destPath := ""
		if len(os.Args) >= 4 {
			destPath = os.Args[3]
		}
		cmdGet(os.Args[2], destPath)

	case "list":
		cmdList()

	case "clean":
		maxAge := DefaultMaxAge
		if len(os.Args) >= 3 {
			days := 0
			if _, err := fmt.Sscanf(os.Args[2], "%d", &days); err == nil {
				maxAge = time.Duration(days) * 24 * time.Hour
			}
		}
		cmdClean(maxAge)

	case "remove":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: remove requires <key> argument\n")
			showUsage()
			os.Exit(1)
		}
		cmdRemove(os.Args[2])

	case "stats":
		cmdStats()

	case "verify":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: verify requires <key> argument\n")
			showUsage()
			os.Exit(1)
		}
		cmdVerify(os.Args[2])

	case "export":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: export requires <output-dir> argument\n")
			showUsage()
			os.Exit(1)
		}
		cmdExport(os.Args[2])

	case "import":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: import requires <input-dir> argument\n")
			showUsage()
			os.Exit(1)
		}
		cmdImport(os.Args[2])

	case "version":
		fmt.Printf("rock-cache version %s\n", Version)
		fmt.Printf("  Build time: %s\n", BuildTime)
		fmt.Printf("  Git commit: %s\n", GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-cache - Artifact caching for ROCK-OS tools

Usage:
  rock-cache <command> [arguments]

Commands:
  store <key> <file>      Store an artifact in cache
  get <key> [dest]        Retrieve an artifact from cache
  list                    List all cached artifacts
  clean [days]            Remove artifacts older than N days (default: 7)
  remove <key>            Remove a specific artifact
  stats                   Show cache statistics
  verify <key>            Verify integrity of cached artifact
  export <dir>            Export cache to directory
  import <dir>            Import cache from directory
  version                 Show version information

Environment Variables:
  ROCK_CACHE_DIR          Cache directory (default: ~/.rock-cache)
  ROCK_VERBOSE            Enable verbose output (true/false)
  ROCK_OUTPUT             Output format (json/text)

Examples:
  rock-cache store kernel-5.15 vmlinuz
  rock-cache get kernel-5.15 /boot/vmlinuz
  rock-cache list
  rock-cache clean 30
  rock-cache stats`)
}

func initializeCacheDir() error {
	// Create cache directories if they don't exist
	dirs := []string{cacheDir, artifactsDir, metadataDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	return nil
}

func cmdStore(key, filePath string) {
	// Validate key
	if !isValidKey(key) {
		fmt.Fprintf(os.Stderr, "Error: invalid key format. Use alphanumeric, dash, underscore, and dot only\n")
		os.Exit(1)
	}

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot access file %s: %v\n", filePath, err)
		os.Exit(1)
	}

	// Calculate file hash
	hash, err := calculateFileHash(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating hash: %v\n", err)
		os.Exit(1)
	}

	// Create cache entry
	entry := &CacheEntry{
		Key:         key,
		Filename:    filepath.Base(filePath),
		Size:        fileInfo.Size(),
		Hash:        hash,
		Timestamp:   time.Now(),
		AccessCount: 0,
		LastAccess:  time.Now(),
	}

	// Copy file to cache
	artifactPath := filepath.Join(artifactsDir, key)
	if err := copyFile(filePath, artifactPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error storing artifact: %v\n", err)
		os.Exit(1)
	}

	// Save metadata
	metadataPath := filepath.Join(metadataDir, key+".json")
	if err := saveMetadata(entry, metadataPath); err != nil {
		// Clean up artifact if metadata save fails
		os.Remove(artifactPath)
		fmt.Fprintf(os.Stderr, "Error saving metadata: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(entry)
	} else {
		fmt.Printf("✅ Cached artifact '%s'\n", key)
		fmt.Printf("   File: %s\n", entry.Filename)
		fmt.Printf("   Size: %s\n", formatSize(entry.Size))
		fmt.Printf("   Hash: %s\n", entry.Hash[:16]+"...")
	}
}

func cmdGet(key, destPath string) {
	// Validate key
	if !isValidKey(key) {
		fmt.Fprintf(os.Stderr, "Error: invalid key format\n")
		os.Exit(1)
	}

	// Load metadata
	metadataPath := filepath.Join(metadataDir, key+".json")
	entry, err := loadMetadata(metadataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: artifact '%s' not found in cache\n", key)
		os.Exit(1)
	}

	// Check if artifact exists
	artifactPath := filepath.Join(artifactsDir, key)
	if _, err := os.Stat(artifactPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: artifact file missing for '%s'\n", key)
		os.Exit(1)
	}

	// Determine destination path
	if destPath == "" {
		destPath = entry.Filename
	}

	// Copy artifact to destination
	if err := copyFile(artifactPath, destPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error retrieving artifact: %v\n", err)
		os.Exit(1)
	}

	// Update access metadata
	entry.AccessCount++
	entry.LastAccess = time.Now()
	saveMetadata(entry, metadataPath)

	if jsonOutput {
		result := map[string]interface{}{
			"key":         key,
			"destination": destPath,
			"size":        entry.Size,
			"hash":        entry.Hash,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("✅ Retrieved artifact '%s' -> %s\n", key, destPath)
		fmt.Printf("   Size: %s\n", formatSize(entry.Size))
		fmt.Printf("   Cached: %s\n", entry.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Access count: %d\n", entry.AccessCount)
	}
}

func cmdList() {
	entries, err := listAllEntries()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing cache: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		if !jsonOutput {
			fmt.Println("No cached artifacts")
		} else {
			fmt.Println("[]")
		}
		return
	}

	// Sort by timestamp (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(entries)
	} else {
		fmt.Println("Cached Artifacts:")
		fmt.Println("================")

		var totalSize int64
		for _, entry := range entries {
			age := time.Since(entry.Timestamp)
			fmt.Printf("\n📦 %s\n", entry.Key)
			fmt.Printf("   File: %s\n", entry.Filename)
			fmt.Printf("   Size: %s\n", formatSize(entry.Size))
			fmt.Printf("   Age: %s\n", formatDuration(age))
			fmt.Printf("   Hash: %s...\n", entry.Hash[:16])
			fmt.Printf("   Access: %d times, last %s\n",
				entry.AccessCount,
				formatDuration(time.Since(entry.LastAccess)))
			totalSize += entry.Size
		}

		fmt.Printf("\nTotal: %d artifacts, %s\n", len(entries), formatSize(totalSize))
	}
}

func cmdClean(maxAge time.Duration) {
	entries, err := listAllEntries()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing cache: %v\n", err)
		os.Exit(1)
	}

	cutoff := time.Now().Add(-maxAge)
	var removed []string
	var totalSize int64

	for _, entry := range entries {
		if entry.Timestamp.Before(cutoff) {
			// Remove artifact and metadata
			artifactPath := filepath.Join(artifactsDir, entry.Key)
			metadataPath := filepath.Join(metadataDir, entry.Key+".json")

			os.Remove(artifactPath)
			os.Remove(metadataPath)

			removed = append(removed, entry.Key)
			totalSize += entry.Size
		}
	}

	if jsonOutput {
		result := map[string]interface{}{
			"removed":      removed,
			"count":        len(removed),
			"size_freed":   totalSize,
			"max_age_days": int(maxAge.Hours() / 24),
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		if len(removed) > 0 {
			fmt.Printf("🧹 Cleaned %d old artifacts (freed %s)\n",
				len(removed), formatSize(totalSize))
			if verboseMode {
				for _, key := range removed {
					fmt.Printf("   - %s\n", key)
				}
			}
		} else {
			fmt.Printf("No artifacts older than %d days\n",
				int(maxAge.Hours()/24))
		}
	}
}

func cmdRemove(key string) {
	// Validate key
	if !isValidKey(key) {
		fmt.Fprintf(os.Stderr, "Error: invalid key format\n")
		os.Exit(1)
	}

	// Check if entry exists
	metadataPath := filepath.Join(metadataDir, key+".json")
	entry, err := loadMetadata(metadataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: artifact '%s' not found\n", key)
		os.Exit(1)
	}

	// Remove artifact and metadata
	artifactPath := filepath.Join(artifactsDir, key)
	os.Remove(artifactPath)
	os.Remove(metadataPath)

	if jsonOutput {
		result := map[string]interface{}{
			"removed": key,
			"size":    entry.Size,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("✅ Removed artifact '%s' (freed %s)\n",
			key, formatSize(entry.Size))
	}
}

func cmdStats() {
	entries, err := listAllEntries()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error gathering stats: %v\n", err)
		os.Exit(1)
	}

	stats := &CacheStats{
		TotalEntries: len(entries),
	}

	if len(entries) > 0 {
		// Find oldest and newest
		oldest := entries[0].Timestamp
		newest := entries[0].Timestamp

		for _, entry := range entries {
			stats.TotalSize += entry.Size
			if entry.Timestamp.Before(oldest) {
				oldest = entry.Timestamp
			}
			if entry.Timestamp.After(newest) {
				newest = entry.Timestamp
			}
		}

		stats.OldestEntry = oldest
		stats.NewestEntry = newest
	}

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(stats)
	} else {
		fmt.Println("Cache Statistics:")
		fmt.Println("================")
		fmt.Printf("Total entries: %d\n", stats.TotalEntries)
		fmt.Printf("Total size: %s\n", formatSize(stats.TotalSize))

		if stats.TotalEntries > 0 {
			fmt.Printf("Oldest entry: %s (%s ago)\n",
				stats.OldestEntry.Format("2006-01-02 15:04:05"),
				formatDuration(time.Since(stats.OldestEntry)))
			fmt.Printf("Newest entry: %s (%s ago)\n",
				stats.NewestEntry.Format("2006-01-02 15:04:05"),
				formatDuration(time.Since(stats.NewestEntry)))
		}

		fmt.Printf("Cache location: %s\n", cacheDir)
	}
}

func cmdVerify(key string) {
	// Load metadata
	metadataPath := filepath.Join(metadataDir, key+".json")
	entry, err := loadMetadata(metadataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: artifact '%s' not found\n", key)
		os.Exit(1)
	}

	// Calculate current hash
	artifactPath := filepath.Join(artifactsDir, key)
	currentHash, err := calculateFileHash(artifactPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error verifying artifact: %v\n", err)
		os.Exit(1)
	}

	// Compare hashes
	valid := currentHash == entry.Hash

	if jsonOutput {
		result := map[string]interface{}{
			"key":           key,
			"valid":         valid,
			"expected_hash": entry.Hash,
			"actual_hash":   currentHash,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		if valid {
			fmt.Printf("✅ Artifact '%s' is valid\n", key)
			fmt.Printf("   Hash: %s\n", currentHash[:32]+"...")
		} else {
			fmt.Printf("❌ Artifact '%s' is corrupted!\n", key)
			fmt.Printf("   Expected: %s\n", entry.Hash[:32]+"...")
			fmt.Printf("   Actual:   %s\n", currentHash[:32]+"...")
			os.Exit(1)
		}
	}
}

func cmdExport(outputDir string) {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Copy entire cache directory
	artifactsOut := filepath.Join(outputDir, "artifacts")
	metadataOut := filepath.Join(outputDir, "metadata")

	if err := copyDir(artifactsDir, artifactsOut); err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting artifacts: %v\n", err)
		os.Exit(1)
	}

	if err := copyDir(metadataDir, metadataOut); err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting metadata: %v\n", err)
		os.Exit(1)
	}

	// Count exported items
	entries, _ := listAllEntries()

	if jsonOutput {
		result := map[string]interface{}{
			"exported_to": outputDir,
			"count":       len(entries),
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("✅ Exported %d artifacts to %s\n", len(entries), outputDir)
	}
}

func cmdImport(inputDir string) {
	// Check if input directory exists
	if _, err := os.Stat(inputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: input directory not found: %s\n", inputDir)
		os.Exit(1)
	}

	artifactsIn := filepath.Join(inputDir, "artifacts")
	metadataIn := filepath.Join(inputDir, "metadata")

	// Import artifacts
	imported := 0
	entries, err := os.ReadDir(metadataIn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading import directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			key := strings.TrimSuffix(entry.Name(), ".json")

			// Copy artifact
			srcArtifact := filepath.Join(artifactsIn, key)
			dstArtifact := filepath.Join(artifactsDir, key)
			if err := copyFile(srcArtifact, dstArtifact); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to import %s: %v\n", key, err)
				continue
			}

			// Copy metadata
			srcMetadata := filepath.Join(metadataIn, entry.Name())
			dstMetadata := filepath.Join(metadataDir, entry.Name())
			if err := copyFile(srcMetadata, dstMetadata); err != nil {
				os.Remove(dstArtifact) // Clean up artifact if metadata fails
				fmt.Fprintf(os.Stderr, "Warning: failed to import metadata for %s: %v\n", key, err)
				continue
			}

			imported++
		}
	}

	if jsonOutput {
		result := map[string]interface{}{
			"imported_from": inputDir,
			"count":         imported,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("✅ Imported %d artifacts from %s\n", imported, inputDir)
	}
}

// Helper functions

func isValidKey(key string) bool {
	// Allow alphanumeric, dash, underscore, and dot
	for _, r := range key {
		if !((r >= 'a' && r <= 'z') ||
			 (r >= 'A' && r <= 'Z') ||
			 (r >= '0' && r <= '9') ||
			 r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return len(key) > 0 && len(key) <= 255
}

func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create destination directory if needed
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func copyDir(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func saveMetadata(entry *CacheEntry, path string) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadMetadata(path string) (*CacheEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

func listAllEntries() ([]*CacheEntry, error) {
	files, err := os.ReadDir(metadataDir)
	if err != nil {
		return nil, err
	}

	var entries []*CacheEntry
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			path := filepath.Join(metadataDir, file.Name())
			entry, err := loadMetadata(path)
			if err != nil {
				continue // Skip corrupted metadata
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
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
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
}