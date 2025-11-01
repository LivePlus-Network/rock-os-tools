package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// ConfigType represents different configuration types
type ConfigType string

const (
	ConfigTypeNode     ConfigType = "node"
	ConfigTypeNetwork  ConfigType = "network"
	ConfigTypeStorage  ConfigType = "storage"
	ConfigTypeSecurity ConfigType = "security"
	ConfigTypeVolcano  ConfigType = "volcano"
	ConfigTypeAll      ConfigType = "all"
)

// Critical paths for rock-init integration
const (
	ConfigDir        = "/config"
	EtcRockDir       = "/etc/rock"
	ConfigKeyPath    = "/config/CONFIG_KEY"     // Line 438 in rock-init
	NodeConfigPath   = "/config/node.yaml"
	SecureConfigPath = "/config/secure.enc"
)

// NodeConfig represents node configuration
type NodeConfig struct {
	Version  string                 `yaml:"version" json:"version"`
	NodeID   string                 `yaml:"node_id" json:"node_id"`
	Hostname string                 `yaml:"hostname" json:"hostname"`
	Role     string                 `yaml:"role" json:"role"`
	Labels   map[string]string      `yaml:"labels" json:"labels"`
	Network  NetworkConfig          `yaml:"network" json:"network"`
	Storage  StorageConfig          `yaml:"storage" json:"storage"`
	Features map[string]interface{} `yaml:"features" json:"features"`
}

// NetworkConfig represents network configuration
type NetworkConfig struct {
	Interface   string   `yaml:"interface" json:"interface"`
	IPAddress   string   `yaml:"ip_address" json:"ip_address"`
	Gateway     string   `yaml:"gateway" json:"gateway"`
	DNS         []string `yaml:"dns" json:"dns"`
	MTU         int      `yaml:"mtu" json:"mtu"`
	BridgeMode  bool     `yaml:"bridge_mode" json:"bridge_mode"`
	VLANs       []int    `yaml:"vlans,omitempty" json:"vlans,omitempty"`
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	RootDevice   string            `yaml:"root_device" json:"root_device"`
	DataDevices  []string          `yaml:"data_devices" json:"data_devices"`
	CacheDevice  string            `yaml:"cache_device,omitempty" json:"cache_device,omitempty"`
	StorageClass string            `yaml:"storage_class" json:"storage_class"`
	Quotas       map[string]string `yaml:"quotas" json:"quotas"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	EncryptionEnabled bool              `yaml:"encryption_enabled" json:"encryption_enabled"`
	KeyManagement     string            `yaml:"key_management" json:"key_management"`
	TLSCert           string            `yaml:"tls_cert,omitempty" json:"tls_cert,omitempty"`
	TLSKey            string            `yaml:"tls_key,omitempty" json:"tls_key,omitempty"`
	CACert            string            `yaml:"ca_cert,omitempty" json:"ca_cert,omitempty"`
	AuthMode          string            `yaml:"auth_mode" json:"auth_mode"`
	Secrets           map[string]string `yaml:"secrets,omitempty" json:"secrets,omitempty"`
}

// VolcanoConfig represents volcano-agent configuration
type VolcanoConfig struct {
	Version       string            `yaml:"version" json:"version"`
	AgentID       string            `yaml:"agent_id" json:"agent_id"`
	ServerURL     string            `yaml:"server_url" json:"server_url"`
	AuthToken     string            `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	HeartbeatSec  int               `yaml:"heartbeat_sec" json:"heartbeat_sec"`
	MaxRetries    int               `yaml:"max_retries" json:"max_retries"`
	Features      []string          `yaml:"features" json:"features"`
	CustomMetrics map[string]string `yaml:"custom_metrics,omitempty" json:"custom_metrics,omitempty"`
}

// ValidationResult represents the result of config validation
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Type     string   `json:"type"`
	Path     string   `json:"path"`
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "generate":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: generate requires a config type\n")
			showUsage()
			os.Exit(1)
		}
		cmdGenerate(os.Args[2])

	case "validate":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: validate requires a config file path\n")
			os.Exit(1)
		}
		cmdValidate(os.Args[2])

	case "encrypt":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: encrypt requires a config file path\n")
			os.Exit(1)
		}
		key := ""
		if len(os.Args) > 3 {
			key = os.Args[3]
		}
		cmdEncrypt(os.Args[2], key)

	case "decrypt":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: decrypt requires an encrypted file path\n")
			os.Exit(1)
		}
		key := ""
		if len(os.Args) > 3 {
			key = os.Args[3]
		}
		cmdDecrypt(os.Args[2], key)

	case "merge":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: merge requires base and override config paths\n")
			os.Exit(1)
		}
		cmdMerge(os.Args[2], os.Args[3])

	case "init":
		cmdInit()

	case "check":
		cmdCheck()

	case "version":
		fmt.Printf("rock-config version %s (built %s, commit %s)\n",
			Version, BuildTime, GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-config - Configuration Management for ROCK-OS

Manages configuration files for ROCK-OS components.
Creates configs at /etc/rock/ and /config/ as required by rock-init.

Usage:
  rock-config generate <type>        Generate default configuration
  rock-config validate <config>      Validate configuration file
  rock-config encrypt <config> [key] Encrypt sensitive configuration
  rock-config decrypt <file> [key]   Decrypt configuration
  rock-config merge <base> <override> Merge configurations
  rock-config init                   Initialize config directories
  rock-config check                  Check config environment
  rock-config version               Show version

Config Types:
  node       Node configuration (/config/node.yaml)
  network    Network settings
  storage    Storage configuration
  security   Security settings (/config/CONFIG_KEY)
  volcano    Volcano agent config
  all        Generate all configs

Examples:
  # Generate default node config
  rock-config generate node > /config/node.yaml

  # Validate configuration
  rock-config validate /config/node.yaml

  # Encrypt sensitive config
  rock-config encrypt /config/security.yaml

  # Initialize config structure
  rock-config init

Environment:
  ROCK_CONFIG_DIR     Config directory (default: /config)
  ROCK_CONFIG_KEY     Encryption key (or read from /config/CONFIG_KEY)
  ROCK_OUTPUT=json    JSON output format
  ROCK_VERBOSE=1      Verbose output

CRITICAL Integration Paths:
  /config/CONFIG_KEY    Encryption key (rock-init line 438)
  /config/node.yaml     Node configuration
  /etc/rock/            Additional configs
  /config/secure.enc    Encrypted sensitive data`)
}

func cmdGenerate(configType string) {
	switch ConfigType(configType) {
	case ConfigTypeNode:
		generateNodeConfig()
	case ConfigTypeNetwork:
		generateNetworkConfig()
	case ConfigTypeStorage:
		generateStorageConfig()
	case ConfigTypeSecurity:
		generateSecurityConfig()
	case ConfigTypeVolcano:
		generateVolcanoConfig()
	case ConfigTypeAll:
		generateAllConfigs()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown config type: %s\n", configType)
		fmt.Fprintln(os.Stderr, "Valid types: node, network, storage, security, volcano, all")
		os.Exit(1)
	}
}

func generateNodeConfig() {
	config := NodeConfig{
		Version:  "1.0",
		NodeID:   generateID("node"),
		Hostname: "rock-node-001",
		Role:     "worker",
		Labels: map[string]string{
			"environment": "production",
			"region":      "us-west",
			"zone":        "us-west-1a",
		},
		Network: NetworkConfig{
			Interface: "eth0",
			IPAddress: "dhcp",
			Gateway:   "auto",
			DNS:       []string{"8.8.8.8", "8.8.4.4"},
			MTU:       1500,
			BridgeMode: false,
		},
		Storage: StorageConfig{
			RootDevice:   "/dev/sda1",
			DataDevices:  []string{"/dev/sdb1"},
			StorageClass: "fast-ssd",
			Quotas: map[string]string{
				"default": "100Gi",
				"system":  "20Gi",
			},
		},
		Features: map[string]interface{}{
			"monitoring": true,
			"logging":    true,
			"debug":      false,
		},
	}

	outputConfig(config, "yaml")
}

func generateNetworkConfig() {
	config := NetworkConfig{
		Interface:  "eth0",
		IPAddress:  "192.168.1.100",
		Gateway:    "192.168.1.1",
		DNS:        []string{"8.8.8.8", "1.1.1.1"},
		MTU:        1500,
		BridgeMode: false,
		VLANs:      []int{100, 200},
	}

	outputConfig(config, "yaml")
}

func generateStorageConfig() {
	config := StorageConfig{
		RootDevice:   "/dev/sda1",
		DataDevices:  []string{"/dev/sdb1", "/dev/sdc1"},
		CacheDevice:  "/dev/nvme0n1",
		StorageClass: "fast-ssd",
		Quotas: map[string]string{
			"default":     "100Gi",
			"system":      "20Gi",
			"user-data":   "500Gi",
			"cache":       "50Gi",
		},
	}

	outputConfig(config, "yaml")
}

func generateSecurityConfig() {
	config := SecurityConfig{
		EncryptionEnabled: true,
		KeyManagement:     "local",
		AuthMode:          "token",
		Secrets: map[string]string{
			"api_key":     generateRandomKey(32),
			"cluster_key": generateRandomKey(32),
		},
	}

	// Don't output secrets in plaintext
	config.Secrets = map[string]string{
		"api_key":     "REDACTED - use encrypt command",
		"cluster_key": "REDACTED - use encrypt command",
	}

	outputConfig(config, "yaml")

	fmt.Fprintln(os.Stderr, "\n⚠️  Security config contains sensitive data!")
	fmt.Fprintln(os.Stderr, "Use 'rock-config encrypt' to secure sensitive fields")
}

func generateVolcanoConfig() {
	config := VolcanoConfig{
		Version:      "1.0",
		AgentID:      generateID("volcano"),
		ServerURL:    "https://volcano-server.rock-os.local:8443",
		AuthToken:    "GENERATED_TOKEN_PLACEHOLDER",
		HeartbeatSec: 30,
		MaxRetries:   3,
		Features:     []string{"metrics", "logs", "events", "health"},
		CustomMetrics: map[string]string{
			"namespace": "rock-os",
			"subsystem": "volcano",
		},
	}

	outputConfig(config, "yaml")
}

func generateAllConfigs() {
	// Create output directory structure
	configDir := getConfigDir()
	etcRockDir := filepath.Join(configDir, "etc", "rock")

	// Create directories
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(etcRockDir, 0755)

	// Generate each config to files
	configs := map[string]func(){
		filepath.Join(configDir, "node.yaml"):        generateNodeConfig,
		filepath.Join(etcRockDir, "network.yaml"):    generateNetworkConfig,
		filepath.Join(etcRockDir, "storage.yaml"):    generateStorageConfig,
		filepath.Join(etcRockDir, "security.yaml"):   generateSecurityConfig,
		filepath.Join(configDir, "volcano.yaml"):     generateVolcanoConfig,
	}

	for path, generator := range configs {
		file, err := os.Create(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", path, err)
			continue
		}

		// Redirect stdout temporarily
		oldStdout := os.Stdout
		os.Stdout = file
		generator()
		os.Stdout = oldStdout
		file.Close()

		fmt.Printf("✅ Generated: %s\n", path)
	}

	// Generate encryption key
	keyPath := filepath.Join(configDir, "CONFIG_KEY")
	key := generateRandomKey(32)
	if err := os.WriteFile(keyPath, []byte(key), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create CONFIG_KEY: %v\n", err)
	} else {
		fmt.Printf("✅ Generated: %s (encryption key)\n", keyPath)
	}

	fmt.Println("\n✅ All configurations generated successfully!")
	fmt.Printf("Config directory: %s\n", configDir)
}

func cmdValidate(configPath string) {
	result := ValidationResult{
		Path:     configPath,
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Cannot read file: %v", err))
		outputValidationResult(result)
		os.Exit(1)
	}

	// Detect config type based on content or path
	configType := detectConfigType(configPath, data)
	result.Type = configType

	// Parse and validate based on type
	switch configType {
	case "node":
		var config NodeConfig
		if err := unmarshalConfig(data, &config); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Parse error: %v", err))
		} else {
			validateNodeConfig(&config, &result)
		}

	case "network":
		var config NetworkConfig
		if err := unmarshalConfig(data, &config); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Parse error: %v", err))
		} else {
			validateNetworkConfig(&config, &result)
		}

	case "storage":
		var config StorageConfig
		if err := unmarshalConfig(data, &config); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Parse error: %v", err))
		} else {
			validateStorageConfig(&config, &result)
		}

	case "security":
		var config SecurityConfig
		if err := unmarshalConfig(data, &config); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Parse error: %v", err))
		} else {
			validateSecurityConfig(&config, &result)
		}

	case "volcano":
		var config VolcanoConfig
		if err := unmarshalConfig(data, &config); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Parse error: %v", err))
		} else {
			validateVolcanoConfig(&config, &result)
		}

	default:
		result.Warnings = append(result.Warnings, "Unknown config type, performing basic validation only")
		// Basic structure validation
		var generic map[string]interface{}
		if err := unmarshalConfig(data, &generic); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Invalid YAML/JSON: %v", err))
		}
	}

	outputValidationResult(result)

	if !result.Valid {
		os.Exit(1)
	}
}

func cmdEncrypt(configPath, key string) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Get encryption key
	encKey := getEncryptionKey(key)

	// Encrypt data
	encrypted, err := encrypt(data, encKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Encryption failed: %v\n", err)
		os.Exit(1)
	}

	// Determine output path
	outputPath := configPath + ".enc"
	if strings.HasSuffix(configPath, ".yaml") || strings.HasSuffix(configPath, ".yml") {
		outputPath = strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".enc"
	}

	// Write encrypted file
	if err := os.WriteFile(outputPath, []byte(encrypted), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write encrypted file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Encrypted: %s\n", outputPath)
	fmt.Printf("   Original: %d bytes\n", len(data))
	fmt.Printf("   Encrypted: %d bytes\n", len(encrypted))
	fmt.Println("\n🔐 Keep your encryption key safe!")
	fmt.Printf("   Key location: %s\n", ConfigKeyPath)
}

func cmdDecrypt(encPath, key string) {
	// Read encrypted file
	encData, err := os.ReadFile(encPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Get encryption key
	encKey := getEncryptionKey(key)

	// Decrypt data
	decrypted, err := decrypt(string(encData), encKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Decryption failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Check that you're using the correct key\n")
		os.Exit(1)
	}

	// Output decrypted data
	fmt.Print(string(decrypted))
}

func cmdMerge(basePath, overridePath string) {
	// Read base config
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading base config: %v\n", err)
		os.Exit(1)
	}

	// Read override config
	overrideData, err := os.ReadFile(overridePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading override config: %v\n", err)
		os.Exit(1)
	}

	// Parse both configs
	var base, override map[string]interface{}
	if err := unmarshalConfig(baseData, &base); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing base config: %v\n", err)
		os.Exit(1)
	}
	if err := unmarshalConfig(overrideData, &override); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing override config: %v\n", err)
		os.Exit(1)
	}

	// Merge configs
	merged := mergeConfigs(base, override)

	// Output merged config
	outputConfig(merged, "yaml")
}

func cmdInit() {
	fmt.Println("Initializing ROCK-OS configuration structure...")

	dirs := []string{
		ConfigDir,
		EtcRockDir,
		filepath.Join(ConfigDir, "backups"),
		filepath.Join(EtcRockDir, "templates"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create %s: %v\n", dir, err)
		} else {
			fmt.Printf("✅ Created: %s\n", dir)
		}
	}

	// Create CONFIG_KEY if it doesn't exist
	if _, err := os.Stat(ConfigKeyPath); os.IsNotExist(err) {
		key := generateRandomKey(32)
		if err := os.WriteFile(ConfigKeyPath, []byte(key), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create CONFIG_KEY: %v\n", err)
		} else {
			fmt.Printf("✅ Created: %s (encryption key)\n", ConfigKeyPath)
		}
	} else {
		fmt.Printf("ℹ️  CONFIG_KEY already exists\n")
	}

	fmt.Println("\n✅ Configuration structure initialized!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Generate configs: rock-config generate all")
	fmt.Println("  2. Edit configs as needed")
	fmt.Println("  3. Validate: rock-config validate /config/node.yaml")
	fmt.Println("  4. Encrypt sensitive data: rock-config encrypt /config/security.yaml")
}

func cmdCheck() {
	fmt.Println("Checking configuration environment...")
	fmt.Println("=" + strings.Repeat("=", 60))

	// Check directories
	dirs := map[string]string{
		ConfigDir:  "Main config directory",
		EtcRockDir: "Rock-specific configs",
	}

	for dir, desc := range dirs {
		if stat, err := os.Stat(dir); err == nil {
			perms := stat.Mode().Perm()
			fmt.Printf("✅ %s: %s (mode: %04o)\n", desc, dir, perms)
		} else {
			fmt.Printf("❌ %s: %s (not found)\n", desc, dir)
		}
	}

	// Check critical files
	files := map[string]string{
		ConfigKeyPath:                  "Encryption key",
		filepath.Join(ConfigDir, "node.yaml"): "Node configuration",
		filepath.Join(ConfigDir, "volcano.yaml"): "Volcano agent config",
	}

	fmt.Println("\nConfiguration Files:")
	for file, desc := range files {
		if stat, err := os.Stat(file); err == nil {
			size := stat.Size()
			fmt.Printf("✅ %s: %s (%d bytes)\n", desc, file, size)
		} else {
			fmt.Printf("⚠️  %s: %s (not found)\n", desc, file)
		}
	}

	// Check environment variables
	fmt.Println("\nEnvironment:")
	envVars := []string{"ROCK_CONFIG_DIR", "ROCK_CONFIG_KEY", "ROCK_OUTPUT"}
	for _, env := range envVars {
		if val := os.Getenv(env); val != "" {
			fmt.Printf("  %s=%s\n", env, val)
		}
	}

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("\nUse 'rock-config init' to initialize missing directories")
}

// Helper functions

func getConfigDir() string {
	if dir := os.Getenv("ROCK_CONFIG_DIR"); dir != "" {
		return dir
	}
	return ConfigDir
}

func getEncryptionKey(key string) []byte {
	if key != "" {
		return []byte(key)
	}

	// Try environment variable
	if envKey := os.Getenv("ROCK_CONFIG_KEY"); envKey != "" {
		return []byte(envKey)
	}

	// Try to read from CONFIG_KEY file
	if data, err := os.ReadFile(ConfigKeyPath); err == nil {
		return []byte(strings.TrimSpace(string(data)))
	}

	// Generate a new key
	newKey := generateRandomKey(32)
	fmt.Fprintf(os.Stderr, "⚠️  No encryption key found, using generated key: %s\n", newKey)
	fmt.Fprintf(os.Stderr, "   Save this key to %s\n", ConfigKeyPath)
	return []byte(newKey)
}

func generateRandomKey(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)[:length]
}

func generateID(prefix string) string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("%s-%x", prefix, bytes)
}

func encrypt(data []byte, key []byte) (string, error) {
	// Create cipher
	hash := sha256.Sum256(key)
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return "", err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Encode to base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(encrypted string, key []byte) ([]byte, error) {
	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}

	// Create cipher
	hash := sha256.Sum256(key)
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func unmarshalConfig(data []byte, v interface{}) error {
	// For now, only support JSON since we don't have yaml package
	// In production, you'd want to add gopkg.in/yaml.v3
	return json.Unmarshal(data, v)
}

func outputConfig(config interface{}, format string) {
	// For now, always output JSON since we don't have yaml package
	// In production, you'd want to add gopkg.in/yaml.v3 for YAML support
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(config)
}

func outputValidationResult(result ValidationResult) {
	if os.Getenv("ROCK_OUTPUT") == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(result)
		return
	}

	// Human readable output
	if result.Valid {
		fmt.Printf("✅ Configuration is valid\n")
		fmt.Printf("   Type: %s\n", result.Type)
		fmt.Printf("   Path: %s\n", result.Path)
	} else {
		fmt.Printf("❌ Configuration is invalid\n")
		fmt.Printf("   Type: %s\n", result.Type)
		fmt.Printf("   Path: %s\n", result.Path)

		if len(result.Errors) > 0 {
			fmt.Println("\nErrors:")
			for _, err := range result.Errors {
				fmt.Printf("  • %s\n", err)
			}
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warn := range result.Warnings {
			fmt.Printf("  ⚠️  %s\n", warn)
		}
	}
}

func detectConfigType(path string, data []byte) string {
	// Check by filename
	base := filepath.Base(path)
	if strings.Contains(base, "node") {
		return "node"
	}
	if strings.Contains(base, "network") {
		return "network"
	}
	if strings.Contains(base, "storage") {
		return "storage"
	}
	if strings.Contains(base, "security") {
		return "security"
	}
	if strings.Contains(base, "volcano") {
		return "volcano"
	}

	// Check by content
	content := string(data)
	if strings.Contains(content, "node_id") {
		return "node"
	}
	if strings.Contains(content, "agent_id") {
		return "volcano"
	}
	if strings.Contains(content, "interface") && strings.Contains(content, "ip_address") {
		return "network"
	}
	if strings.Contains(content, "root_device") {
		return "storage"
	}
	if strings.Contains(content, "encryption_enabled") {
		return "security"
	}

	return "unknown"
}

// Validation functions

func validateNodeConfig(config *NodeConfig, result *ValidationResult) {
	if config.Version == "" {
		result.Errors = append(result.Errors, "Version is required")
		result.Valid = false
	}

	if config.NodeID == "" {
		result.Errors = append(result.Errors, "NodeID is required")
		result.Valid = false
	}

	if config.Hostname == "" {
		result.Errors = append(result.Errors, "Hostname is required")
		result.Valid = false
	}

	if config.Role != "master" && config.Role != "worker" && config.Role != "edge" {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Unusual role: %s (expected: master, worker, or edge)", config.Role))
	}

	// Validate nested configs
	validateNetworkConfig(&config.Network, result)
	validateStorageConfig(&config.Storage, result)
}

func validateNetworkConfig(config *NetworkConfig, result *ValidationResult) {
	if config.Interface == "" {
		result.Errors = append(result.Errors, "Network interface is required")
		result.Valid = false
	}

	if config.IPAddress == "" {
		result.Errors = append(result.Errors, "IP address configuration is required")
		result.Valid = false
	}

	if config.MTU < 1280 || config.MTU > 9000 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Unusual MTU value: %d (typical range: 1280-9000)", config.MTU))
	}

	if len(config.DNS) == 0 {
		result.Warnings = append(result.Warnings, "No DNS servers configured")
	}
}

func validateStorageConfig(config *StorageConfig, result *ValidationResult) {
	if config.RootDevice == "" {
		result.Errors = append(result.Errors, "Root device is required")
		result.Valid = false
	}

	if len(config.DataDevices) == 0 {
		result.Warnings = append(result.Warnings, "No data devices configured")
	}

	if config.StorageClass == "" {
		result.Warnings = append(result.Warnings, "Storage class not specified")
	}
}

func validateSecurityConfig(config *SecurityConfig, result *ValidationResult) {
	if config.KeyManagement == "" {
		result.Errors = append(result.Errors, "Key management mode is required")
		result.Valid = false
	}

	if config.AuthMode == "" {
		result.Errors = append(result.Errors, "Authentication mode is required")
		result.Valid = false
	}

	if config.EncryptionEnabled && config.KeyManagement == "none" {
		result.Errors = append(result.Errors, "Encryption enabled but key management is 'none'")
		result.Valid = false
	}

	if config.TLSCert != "" && config.TLSKey == "" {
		result.Errors = append(result.Errors, "TLS cert provided but key is missing")
		result.Valid = false
	}
}

func validateVolcanoConfig(config *VolcanoConfig, result *ValidationResult) {
	if config.Version == "" {
		result.Errors = append(result.Errors, "Version is required")
		result.Valid = false
	}

	if config.AgentID == "" {
		result.Errors = append(result.Errors, "AgentID is required")
		result.Valid = false
	}

	if config.ServerURL == "" {
		result.Errors = append(result.Errors, "ServerURL is required")
		result.Valid = false
	}

	if config.HeartbeatSec < 10 || config.HeartbeatSec > 300 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Unusual heartbeat interval: %d (typical: 10-300)", config.HeartbeatSec))
	}

	if config.MaxRetries < 1 || config.MaxRetries > 10 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Unusual max retries: %d (typical: 1-10)", config.MaxRetries))
	}
}

func mergeConfigs(base, override map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Copy base
	for k, v := range base {
		merged[k] = v
	}

	// Apply overrides
	for k, v := range override {
		if baseVal, exists := merged[k]; exists {
			// Recursively merge maps
			if baseMap, ok := baseVal.(map[string]interface{}); ok {
				if overrideMap, ok := v.(map[string]interface{}); ok {
					merged[k] = mergeConfigs(baseMap, overrideMap)
					continue
				}
			}
		}
		merged[k] = v
	}

	return merged
}