package main

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Critical path for rock-init integration
const (
	ConfigKeyPath = "/config/CONFIG_KEY" // Line 438 in rock-init - MUST BE EXACT!
	DefaultKeyDir = "/etc/rock/keys"
	BackupKeyDir  = "/config/keys"
)

// KeyType represents different key types
type KeyType string

const (
	KeyTypeAES     KeyType = "aes"
	KeyTypeRSA     KeyType = "rsa"
	KeyTypeED25519 KeyType = "ed25519"
)

// KeyInfo represents information about a generated key
type KeyInfo struct {
	Type        string    `json:"type"`
	Algorithm   string    `json:"algorithm"`
	Size        int       `json:"size"`
	Path        string    `json:"path"`
	Fingerprint string    `json:"fingerprint"`
	Created     time.Time `json:"created"`
	Purpose     string    `json:"purpose"`
}

// SignatureInfo represents signature information
type SignatureInfo struct {
	Algorithm   string    `json:"algorithm"`
	KeyID       string    `json:"key_id"`
	Signature   string    `json:"signature"`
	Hash        string    `json:"hash"`
	SignedAt    time.Time `json:"signed_at"`
	SignedFile  string    `json:"signed_file"`
	Valid       bool      `json:"valid,omitempty"`
}

// SecurityReport represents a security check report
type SecurityReport struct {
	Timestamp   time.Time          `json:"timestamp"`
	ConfigKey   ConfigKeyStatus    `json:"config_key"`
	Keys        []KeyInfo          `json:"keys"`
	Permissions map[string]string  `json:"permissions"`
	Issues      []string           `json:"issues"`
	Warnings    []string           `json:"warnings"`
}

// ConfigKeyStatus represents CONFIG_KEY status
type ConfigKeyStatus struct {
	Exists      bool   `json:"exists"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"`
	Valid       bool   `json:"valid"`
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "keygen":
		if len(os.Args) < 3 {
			// Default to AES for CONFIG_KEY
			cmdKeygen("aes", "")
		} else {
			purpose := ""
			if len(os.Args) > 3 {
				purpose = os.Args[3]
			}
			cmdKeygen(os.Args[2], purpose)
		}

	case "sign":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: sign requires a file path\n")
			os.Exit(1)
		}
		keyPath := ""
		if len(os.Args) > 3 {
			keyPath = os.Args[3]
		}
		cmdSign(os.Args[2], keyPath)

	case "verify":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: verify requires a file path\n")
			os.Exit(1)
		}
		sigPath := ""
		keyPath := ""
		if len(os.Args) > 3 {
			sigPath = os.Args[3]
		}
		if len(os.Args) > 4 {
			keyPath = os.Args[4]
		}
		cmdVerify(os.Args[2], sigPath, keyPath)

	case "hash":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: hash requires a file path\n")
			os.Exit(1)
		}
		cmdHash(os.Args[2])

	case "encrypt":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: encrypt requires a file path\n")
			os.Exit(1)
		}
		keyPath := ""
		if len(os.Args) > 3 {
			keyPath = os.Args[3]
		}
		cmdEncrypt(os.Args[2], keyPath)

	case "decrypt":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: decrypt requires a file path\n")
			os.Exit(1)
		}
		keyPath := ""
		if len(os.Args) > 3 {
			keyPath = os.Args[3]
		}
		cmdDecrypt(os.Args[2], keyPath)

	case "check":
		cmdCheck()

	case "init":
		cmdInit()

	case "rotate":
		cmdRotate()

	case "export":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: export requires a key type\n")
			os.Exit(1)
		}
		cmdExport(os.Args[2])

	case "version":
		fmt.Printf("rock-security version %s (built %s, commit %s)\n",
			Version, BuildTime, GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-security - Security Operations for ROCK-OS

Manages encryption keys, digital signatures, and security operations.
CRITICAL: Places CONFIG_KEY at /config/CONFIG_KEY for rock-init.

Usage:
  rock-security keygen [type] [purpose]  Generate encryption keys
  rock-security sign <file> [key]        Sign artifacts
  rock-security verify <file> [sig] [key] Verify signatures
  rock-security hash <file>              Calculate file hashes
  rock-security encrypt <file> [key]     Encrypt files
  rock-security decrypt <file> [key]     Decrypt files
  rock-security check                    Security environment check
  rock-security init                     Initialize security
  rock-security rotate                   Rotate CONFIG_KEY
  rock-security export <type>            Export public keys
  rock-security version                  Show version

Key Types:
  aes        AES-256 symmetric key (default for CONFIG_KEY)
  rsa        RSA-4096 asymmetric keypair
  ed25519    ED25519 signing keypair

Examples:
  # Generate CONFIG_KEY for rock-init
  rock-security keygen aes

  # Generate signing keypair
  rock-security keygen ed25519 signing

  # Sign an artifact
  rock-security sign initrd.cpio.gz

  # Verify a signature
  rock-security verify initrd.cpio.gz

  # Check security status
  rock-security check

Environment:
  ROCK_KEY_DIR        Key directory (default: /etc/rock/keys)
  ROCK_KEY_TYPE       Default key type (aes/rsa/ed25519)
  ROCK_OUTPUT=json    JSON output format

CRITICAL Integration:
  /config/CONFIG_KEY    Main encryption key (rock-init line 438)
  /etc/rock/keys/       Key storage directory
  *.sig                 Signature files
  *.pub                 Public key files`)
}

func cmdKeygen(keyType string, purpose string) {
	switch KeyType(keyType) {
	case KeyTypeAES, "":
		generateAESKey(purpose)
	case KeyTypeRSA:
		generateRSAKey(purpose)
	case KeyTypeED25519:
		generateED25519Key(purpose)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown key type: %s\n", keyType)
		fmt.Fprintln(os.Stderr, "Valid types: aes, rsa, ed25519")
		os.Exit(1)
	}
}

func generateAESKey(purpose string) {
	// Generate 256-bit key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating key: %v\n", err)
		os.Exit(1)
	}

	// Encode as base64
	encoded := base64.StdEncoding.EncodeToString(key)

	// Determine path
	var keyPath string
	if purpose == "" || purpose == "config" || purpose == "CONFIG_KEY" {
		// This is the main CONFIG_KEY for rock-init
		keyPath = ConfigKeyPath

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Other purpose keys go in key directory
		keyDir := getKeyDir()
		if err := os.MkdirAll(keyDir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating key directory: %v\n", err)
			os.Exit(1)
		}
		keyPath = filepath.Join(keyDir, fmt.Sprintf("%s.key", purpose))
	}

	// Write key with secure permissions
	if err := os.WriteFile(keyPath, []byte(encoded), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing key: %v\n", err)
		os.Exit(1)
	}

	// Calculate fingerprint
	hash := sha256.Sum256(key)
	fingerprint := hex.EncodeToString(hash[:8])

	// Create key info
	info := KeyInfo{
		Type:        "symmetric",
		Algorithm:   "AES-256",
		Size:        256,
		Path:        keyPath,
		Fingerprint: fingerprint,
		Created:     time.Now(),
		Purpose:     purpose,
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(info)
	} else {
		fmt.Printf("✅ Generated AES-256 key\n")
		fmt.Printf("   Path: %s\n", keyPath)
		fmt.Printf("   Size: %d bits\n", info.Size)
		fmt.Printf("   Fingerprint: %s\n", fingerprint)
		if keyPath == ConfigKeyPath {
			fmt.Println("\n🔐 CRITICAL: This is the CONFIG_KEY for rock-init!")
			fmt.Println("   Keep this key safe - it's required for boot")
		}
	}
}

func generateRSAKey(purpose string) {
	// Generate RSA keypair
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating RSA key: %v\n", err)
		os.Exit(1)
	}

	// Encode private key
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	// Encode public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding public key: %v\n", err)
		os.Exit(1)
	}
	publicKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	// Determine paths
	keyDir := getKeyDir()
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating key directory: %v\n", err)
		os.Exit(1)
	}

	name := purpose
	if name == "" {
		name = "rsa"
	}

	privateKeyPath := filepath.Join(keyDir, fmt.Sprintf("%s.key", name))
	publicKeyPath := filepath.Join(keyDir, fmt.Sprintf("%s.pub", name))

	// Write private key
	privateFile, err := os.OpenFile(privateKeyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating private key file: %v\n", err)
		os.Exit(1)
	}
	pem.Encode(privateFile, privateKeyPEM)
	privateFile.Close()

	// Write public key
	publicFile, err := os.OpenFile(publicKeyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating public key file: %v\n", err)
		os.Exit(1)
	}
	pem.Encode(publicFile, publicKeyPEM)
	publicFile.Close()

	// Calculate fingerprint
	hash := sha256.Sum256(publicKeyBytes)
	fingerprint := hex.EncodeToString(hash[:8])

	info := KeyInfo{
		Type:        "asymmetric",
		Algorithm:   "RSA",
		Size:        4096,
		Path:        privateKeyPath,
		Fingerprint: fingerprint,
		Created:     time.Now(),
		Purpose:     purpose,
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(info)
	} else {
		fmt.Printf("✅ Generated RSA-4096 keypair\n")
		fmt.Printf("   Private: %s\n", privateKeyPath)
		fmt.Printf("   Public:  %s\n", publicKeyPath)
		fmt.Printf("   Fingerprint: %s\n", fingerprint)
	}
}

func generateED25519Key(purpose string) {
	// Generate ED25519 keypair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating ED25519 key: %v\n", err)
		os.Exit(1)
	}

	// Encode private key
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding private key: %v\n", err)
		os.Exit(1)
	}
	privateKeyPEM := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	// Encode public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding public key: %v\n", err)
		os.Exit(1)
	}
	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	// Determine paths
	keyDir := getKeyDir()
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating key directory: %v\n", err)
		os.Exit(1)
	}

	name := purpose
	if name == "" {
		name = "ed25519"
	}

	privateKeyPath := filepath.Join(keyDir, fmt.Sprintf("%s.key", name))
	publicKeyPath := filepath.Join(keyDir, fmt.Sprintf("%s.pub", name))

	// Write private key
	privateFile, err := os.OpenFile(privateKeyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating private key file: %v\n", err)
		os.Exit(1)
	}
	pem.Encode(privateFile, privateKeyPEM)
	privateFile.Close()

	// Write public key
	publicFile, err := os.OpenFile(publicKeyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating public key file: %v\n", err)
		os.Exit(1)
	}
	pem.Encode(publicFile, publicKeyPEM)
	publicFile.Close()

	// Calculate fingerprint
	hash := sha256.Sum256(publicKey)
	fingerprint := hex.EncodeToString(hash[:8])

	info := KeyInfo{
		Type:        "signing",
		Algorithm:   "ED25519",
		Size:        256,
		Path:        privateKeyPath,
		Fingerprint: fingerprint,
		Created:     time.Now(),
		Purpose:     purpose,
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(info)
	} else {
		fmt.Printf("✅ Generated ED25519 signing keypair\n")
		fmt.Printf("   Private: %s\n", privateKeyPath)
		fmt.Printf("   Public:  %s\n", publicKeyPath)
		fmt.Printf("   Fingerprint: %s\n", fingerprint)
	}
}

func cmdSign(filePath string, keyPath string) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Calculate hash
	hash := sha256.Sum256(data)

	// Find key if not specified
	if keyPath == "" {
		keyPath = findSigningKey()
	}

	// Read key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		os.Exit(1)
	}

	// Parse key
	block, _ := pem.Decode(keyData)
	if block == nil {
		fmt.Fprintf(os.Stderr, "Error: invalid PEM key\n")
		os.Exit(1)
	}

	var signature []byte
	var algorithm string
	var keyID string

	// Try ED25519
	if strings.Contains(block.Type, "PRIVATE") {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			if ed25519Key, ok := key.(ed25519.PrivateKey); ok {
				signature = ed25519.Sign(ed25519Key, hash[:])
				algorithm = "ED25519"
				pubHash := sha256.Sum256(ed25519Key.Public().(ed25519.PublicKey))
				keyID = hex.EncodeToString(pubHash[:8])
			}
		}
	}

	// Try RSA
	if signature == nil && strings.Contains(block.Type, "RSA") {
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err == nil {
			signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error signing with RSA: %v\n", err)
				os.Exit(1)
			}
			algorithm = "RSA-PKCS1-SHA256"
			pubBytes, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
			pubHash := sha256.Sum256(pubBytes)
			keyID = hex.EncodeToString(pubHash[:8])
		}
	}

	if signature == nil {
		fmt.Fprintf(os.Stderr, "Error: unable to sign with provided key\n")
		os.Exit(1)
	}

	// Create signature info
	sigInfo := SignatureInfo{
		Algorithm:  algorithm,
		KeyID:      keyID,
		Signature:  base64.StdEncoding.EncodeToString(signature),
		Hash:       hex.EncodeToString(hash[:]),
		SignedAt:   time.Now(),
		SignedFile: filePath,
	}

	// Write signature file
	sigPath := filePath + ".sig"
	sigData, _ := json.MarshalIndent(sigInfo, "", "  ")
	if err := os.WriteFile(sigPath, sigData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing signature: %v\n", err)
		os.Exit(1)
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(sigInfo)
	} else {
		fmt.Printf("✅ Signed: %s\n", filePath)
		fmt.Printf("   Algorithm: %s\n", algorithm)
		fmt.Printf("   Key ID: %s\n", keyID)
		fmt.Printf("   Signature: %s\n", sigPath)
		fmt.Printf("   Hash: %s\n", sigInfo.Hash[:16]+"...")
	}
}

func cmdVerify(filePath string, sigPath string, keyPath string) {
	// Default signature path
	if sigPath == "" {
		sigPath = filePath + ".sig"
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Read signature
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading signature: %v\n", err)
		os.Exit(1)
	}

	// Parse signature info
	var sigInfo SignatureInfo
	if err := json.Unmarshal(sigData, &sigInfo); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing signature: %v\n", err)
		os.Exit(1)
	}

	// Calculate hash
	hash := sha256.Sum256(data)
	hashHex := hex.EncodeToString(hash[:])

	// Verify hash matches
	if hashHex != sigInfo.Hash {
		fmt.Fprintf(os.Stderr, "❌ Hash mismatch!\n")
		fmt.Fprintf(os.Stderr, "   Expected: %s\n", sigInfo.Hash)
		fmt.Fprintf(os.Stderr, "   Got:      %s\n", hashHex)
		os.Exit(1)
	}

	// Find key if not specified
	if keyPath == "" {
		keyPath = findPublicKey(sigInfo.KeyID)
	}

	// Read key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		os.Exit(1)
	}

	// Parse key
	block, _ := pem.Decode(keyData)
	if block == nil {
		fmt.Fprintf(os.Stderr, "Error: invalid PEM key\n")
		os.Exit(1)
	}

	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(sigInfo.Signature)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding signature: %v\n", err)
		os.Exit(1)
	}

	var valid bool

	// Verify based on algorithm
	switch sigInfo.Algorithm {
	case "ED25519":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing ED25519 key: %v\n", err)
			os.Exit(1)
		}
		if ed25519Key, ok := key.(ed25519.PublicKey); ok {
			valid = ed25519.Verify(ed25519Key, hash[:], signature)
		}

	case "RSA-PKCS1-SHA256":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			// Try PKCS1 format
			key, err = x509.ParsePKCS1PublicKey(block.Bytes)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing RSA key: %v\n", err)
			os.Exit(1)
		}
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			err = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], signature)
			valid = (err == nil)
		}

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown algorithm: %s\n", sigInfo.Algorithm)
		os.Exit(1)
	}

	sigInfo.Valid = valid

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(sigInfo)
	} else {
		if valid {
			fmt.Printf("✅ Signature VALID\n")
			fmt.Printf("   File: %s\n", filePath)
			fmt.Printf("   Algorithm: %s\n", sigInfo.Algorithm)
			fmt.Printf("   Key ID: %s\n", sigInfo.KeyID)
			fmt.Printf("   Signed: %s\n", sigInfo.SignedAt.Format(time.RFC3339))
		} else {
			fmt.Printf("❌ Signature INVALID\n")
			fmt.Printf("   File: %s\n", filePath)
			os.Exit(1)
		}
	}

	if !valid {
		os.Exit(1)
	}
}

func cmdHash(filePath string) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Calculate hashes
	sha256Hash := sha256.Sum256(data)
	sha512Hash := sha512.Sum512(data)

	if os.Getenv("ROCK_OUTPUT") == "json" {
		result := map[string]string{
			"file":   filePath,
			"sha256": hex.EncodeToString(sha256Hash[:]),
			"sha512": hex.EncodeToString(sha512Hash[:]),
			"size":   fmt.Sprintf("%d", len(data)),
		}
		outputJSON(result)
	} else {
		fmt.Printf("File: %s\n", filePath)
		fmt.Printf("Size: %d bytes\n", len(data))
		fmt.Printf("SHA256: %s\n", hex.EncodeToString(sha256Hash[:]))
		fmt.Printf("SHA512: %s\n", hex.EncodeToString(sha512Hash[:]))
	}
}

func cmdEncrypt(filePath string, keyPath string) {
	// Read file
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Get key
	if keyPath == "" {
		keyPath = ConfigKeyPath
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		os.Exit(1)
	}

	// Decode key
	key, err := base64.StdEncoding.DecodeString(string(keyData))
	if err != nil {
		// Try raw key
		key = keyData
	}

	if len(key) != 32 {
		// Hash to get 32 bytes
		hash := sha256.Sum256(key)
		key = hash[:]
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cipher: %v\n", err)
		os.Exit(1)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating GCM: %v\n", err)
		os.Exit(1)
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating nonce: %v\n", err)
		os.Exit(1)
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Write encrypted file
	encPath := filePath + ".enc"
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	if err := os.WriteFile(encPath, []byte(encoded), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing encrypted file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Encrypted: %s\n", encPath)
	fmt.Printf("   Original: %d bytes\n", len(plaintext))
	fmt.Printf("   Encrypted: %d bytes\n", len(encoded))
	fmt.Printf("   Key: %s\n", keyPath)
}

func cmdDecrypt(encPath string, keyPath string) {
	// Read encrypted file
	encData, err := os.ReadFile(encPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Get key
	if keyPath == "" {
		keyPath = ConfigKeyPath
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		os.Exit(1)
	}

	// Decode key
	key, err := base64.StdEncoding.DecodeString(string(keyData))
	if err != nil {
		// Try raw key
		key = keyData
	}

	if len(key) != 32 {
		// Hash to get 32 bytes
		hash := sha256.Sum256(key)
		key = hash[:]
	}

	// Decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(string(encData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding encrypted data: %v\n", err)
		os.Exit(1)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cipher: %v\n", err)
		os.Exit(1)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating GCM: %v\n", err)
		os.Exit(1)
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		fmt.Fprintf(os.Stderr, "Error: ciphertext too short\n")
		os.Exit(1)
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decrypting: %v\n", err)
		os.Exit(1)
	}

	// Output
	fmt.Print(string(plaintext))
}

func cmdCheck() {
	report := SecurityReport{
		Timestamp:   time.Now(),
		Keys:        []KeyInfo{},
		Permissions: make(map[string]string),
		Issues:      []string{},
		Warnings:    []string{},
	}

	// Check CONFIG_KEY
	if stat, err := os.Stat(ConfigKeyPath); err == nil {
		report.ConfigKey.Exists = true
		report.ConfigKey.Path = ConfigKeyPath
		report.ConfigKey.Size = stat.Size()
		report.ConfigKey.Permissions = fmt.Sprintf("%04o", stat.Mode().Perm())

		// Check permissions
		if stat.Mode().Perm() != 0600 {
			report.Issues = append(report.Issues,
				fmt.Sprintf("CONFIG_KEY has insecure permissions: %04o (should be 0600)",
					stat.Mode().Perm()))
		}

		// Check size
		if stat.Size() < 32 {
			report.Issues = append(report.Issues, "CONFIG_KEY is too small")
		} else {
			report.ConfigKey.Valid = true
		}
	} else {
		report.ConfigKey.Exists = false
		report.ConfigKey.Path = ConfigKeyPath
		report.Issues = append(report.Issues, "CONFIG_KEY not found - rock-init will fail!")
	}

	// Check key directory
	keyDir := getKeyDir()
	if stat, err := os.Stat(keyDir); err == nil {
		report.Permissions[keyDir] = fmt.Sprintf("%04o", stat.Mode().Perm())

		// List keys
		files, _ := os.ReadDir(keyDir)
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".key") || strings.HasSuffix(file.Name(), ".pub") {
				path := filepath.Join(keyDir, file.Name())
				info, _ := file.Info()

				keyInfo := KeyInfo{
					Path: path,
					Created: info.ModTime(),
				}

				if strings.HasSuffix(file.Name(), ".pub") {
					keyInfo.Type = "public"
				} else {
					keyInfo.Type = "private"
				}

				report.Keys = append(report.Keys, keyInfo)

				// Check private key permissions
				if strings.HasSuffix(file.Name(), ".key") && info.Mode().Perm() != 0600 {
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("Private key %s has permissive mode: %04o",
							file.Name(), info.Mode().Perm()))
				}
			}
		}
	}

	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(report)
	} else {
		fmt.Println("Security Environment Check")
		fmt.Println("=" + strings.Repeat("=", 60))

		// CONFIG_KEY status
		if report.ConfigKey.Exists {
			if report.ConfigKey.Valid {
				fmt.Printf("✅ CONFIG_KEY: %s (%d bytes, mode %s)\n",
					ConfigKeyPath, report.ConfigKey.Size, report.ConfigKey.Permissions)
			} else {
				fmt.Printf("⚠️  CONFIG_KEY: Issues detected\n")
			}
		} else {
			fmt.Printf("❌ CONFIG_KEY: NOT FOUND at %s\n", ConfigKeyPath)
			fmt.Println("   CRITICAL: rock-init requires this key!")
		}

		// Keys
		if len(report.Keys) > 0 {
			fmt.Printf("\nKeys Found: %d\n", len(report.Keys))
			for _, key := range report.Keys {
				fmt.Printf("  • %s (%s)\n", filepath.Base(key.Path), key.Type)
			}
		} else {
			fmt.Println("\nNo additional keys found")
		}

		// Issues and warnings
		if len(report.Issues) > 0 {
			fmt.Println("\n❌ Issues:")
			for _, issue := range report.Issues {
				fmt.Printf("  • %s\n", issue)
			}
		}

		if len(report.Warnings) > 0 {
			fmt.Println("\n⚠️  Warnings:")
			for _, warning := range report.Warnings {
				fmt.Printf("  • %s\n", warning)
			}
		}

		if len(report.Issues) == 0 && len(report.Warnings) == 0 {
			fmt.Println("\n✅ Security environment is properly configured")
		}
	}

	if len(report.Issues) > 0 {
		os.Exit(1)
	}
}

func cmdInit() {
	fmt.Println("Initializing security environment...")

	// Create directories
	dirs := []string{
		filepath.Dir(ConfigKeyPath),
		DefaultKeyDir,
		BackupKeyDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create %s: %v\n", dir, err)
		} else {
			fmt.Printf("✅ Created: %s\n", dir)
		}
	}

	// Generate CONFIG_KEY if missing
	if _, err := os.Stat(ConfigKeyPath); os.IsNotExist(err) {
		fmt.Println("\nGenerating CONFIG_KEY...")
		generateAESKey("config")
	} else {
		fmt.Printf("ℹ️  CONFIG_KEY already exists at %s\n", ConfigKeyPath)
	}

	// Generate default signing key
	signingKeyPath := filepath.Join(DefaultKeyDir, "signing.key")
	if _, err := os.Stat(signingKeyPath); os.IsNotExist(err) {
		fmt.Println("\nGenerating default signing key...")
		generateED25519Key("signing")
	}

	fmt.Println("\n✅ Security environment initialized!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Backup CONFIG_KEY to a secure location")
	fmt.Println("  2. Sign critical artifacts: rock-security sign <file>")
	fmt.Println("  3. Export public keys: rock-security export signing")
}

func cmdRotate() {
	fmt.Println("Rotating CONFIG_KEY...")

	// Backup existing key
	if data, err := os.ReadFile(ConfigKeyPath); err == nil {
		backupPath := fmt.Sprintf("%s.backup.%d", ConfigKeyPath, time.Now().Unix())
		if err := os.WriteFile(backupPath, data, 0600); err == nil {
			fmt.Printf("✅ Backed up existing key to: %s\n", backupPath)
		}
	}

	// Generate new key
	generateAESKey("config")

	fmt.Println("\n⚠️  IMPORTANT: Update all systems with the new CONFIG_KEY")
	fmt.Println("   Old encrypted data will need to be re-encrypted")
}

func cmdExport(keyType string) {
	keyDir := getKeyDir()

	// Find public key
	pubKeyPath := filepath.Join(keyDir, fmt.Sprintf("%s.pub", keyType))
	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: public key not found: %s\n", pubKeyPath)
		os.Exit(1)
	}

	// Read and output
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(string(data))
}

// Helper functions

func getKeyDir() string {
	if dir := os.Getenv("ROCK_KEY_DIR"); dir != "" {
		return dir
	}
	return DefaultKeyDir
}

func findSigningKey() string {
	keyDir := getKeyDir()

	// Look for signing keys in order of preference
	candidates := []string{
		filepath.Join(keyDir, "signing.key"),
		filepath.Join(keyDir, "ed25519.key"),
		filepath.Join(keyDir, "rsa.key"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	fmt.Fprintf(os.Stderr, "Error: no signing key found\n")
	fmt.Fprintf(os.Stderr, "Generate one with: rock-security keygen ed25519 signing\n")
	os.Exit(1)
	return ""
}

func findPublicKey(keyID string) string {
	keyDir := getKeyDir()

	// Try to find matching public key
	files, err := os.ReadDir(keyDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading key directory: %v\n", err)
		os.Exit(1)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".pub") {
			path := filepath.Join(keyDir, file.Name())
			// Check if this key matches the ID
			if keyData, err := os.ReadFile(path); err == nil {
				block, _ := pem.Decode(keyData)
				if block != nil {
					if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
						var pubHash [32]byte
						if ed25519Key, ok := key.(ed25519.PublicKey); ok {
							pubHash = sha256.Sum256(ed25519Key)
						} else if rsaKey, ok := key.(*rsa.PublicKey); ok {
							if pubBytes, err := x509.MarshalPKIXPublicKey(rsaKey); err == nil {
								pubHash = sha256.Sum256(pubBytes)
							}
						}
						if hex.EncodeToString(pubHash[:8]) == keyID {
							return path
						}
					}
				}
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Error: no public key found for ID: %s\n", keyID)
	os.Exit(1)
	return ""
}

func outputJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(data)
}