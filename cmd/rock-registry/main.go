package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	Version   = "1.0.0"
	BuildTime = "2025-01-01T00:00:00Z"
	GitCommit = "dev"

	// Default registry location
	DefaultRegistryDir = ".rock-registry"
	RegistryFile       = "registry.json"
	ComponentsDir      = "components"

	// Component types
	TypeBinary   = "binary"
	TypeLibrary  = "library"
	TypeConfig   = "config"
	TypeKernel   = "kernel"
	TypeInitrd   = "initrd"
	TypeTool     = "tool"
	TypeRuntime  = "runtime"
)

// Component represents a registered ROCK-OS component
type Component struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Type         string            `json:"type"`
	Description  string            `json:"description"`
	Path         string            `json:"path,omitempty"`
	URL          string            `json:"url,omitempty"`
	Hash         string            `json:"hash,omitempty"`
	Size         int64             `json:"size,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Registered   time.Time         `json:"registered"`
	Updated      time.Time         `json:"updated"`
}

// Registry represents the component registry
type Registry struct {
	Version    string                `json:"version"`
	Components map[string]*Component `json:"components"`
	Updated    time.Time             `json:"updated"`
}

// SearchResult represents a component search result
type SearchResult struct {
	Component *Component `json:"component"`
	Score     int        `json:"score"`
	Matches   []string   `json:"matches"`
}

var (
	registryDir  string
	registryPath string
	jsonOutput   bool
	verboseMode  bool
)

// Built-in components for ROCK-OS
var builtInComponents = map[string]*Component{
	"rock-init": {
		Name:        "rock-init",
		Version:     "1.0.0",
		Type:        TypeBinary,
		Description: "ROCK-OS initialization system - first process (PID 1)",
		Path:        "/sbin/init",
		Dependencies: []string{"busybox", "libc"},
		Tags:        []string{"core", "init", "boot", "essential"},
		Metadata: map[string]string{
			"source":     "rock-os/init",
			"target":     "x86_64-unknown-linux-musl",
			"static":     "true",
			"entry":      "main",
			"importance": "critical",
		},
	},
	"rock-manager": {
		Name:        "rock-manager",
		Version:     "1.0.0",
		Type:        TypeBinary,
		Description: "ROCK-OS service manager - handles system services",
		Path:        "/usr/bin/rock-manager",
		Dependencies: []string{"rock-init", "libc"},
		Tags:        []string{"core", "services", "manager", "essential"},
		Metadata: map[string]string{
			"source":     "rock-os/manager",
			"target":     "x86_64-unknown-linux-musl",
			"static":     "true",
			"importance": "critical",
		},
	},
	"volcano-agent": {
		Name:        "volcano-agent",
		Version:     "1.0.0",
		Type:        TypeBinary,
		Description: "Volcano distributed system agent",
		Path:        "/usr/bin/volcano-agent",
		Dependencies: []string{"rock-init", "rock-manager", "libssl", "libcrypto"},
		Tags:        []string{"distributed", "agent", "volcano", "networking"},
		Metadata: map[string]string{
			"source":     "rock-os/agent",
			"target":     "x86_64-unknown-linux-musl",
			"static":     "false",
			"port":      "8080",
			"importance": "high",
		},
	},
	"busybox": {
		Name:        "busybox",
		Version:     "1.35.0",
		Type:        TypeBinary,
		Description: "Multi-call binary providing essential Unix utilities",
		Path:        "/bin/busybox",
		Tags:        []string{"utilities", "shell", "essential", "posix"},
		Metadata: map[string]string{
			"provides":   "sh,ls,cat,echo,mkdir,rm,cp,mv,chmod,chown",
			"static":     "true",
			"importance": "critical",
		},
	},
	"musl-libc": {
		Name:        "musl-libc",
		Version:     "1.2.3",
		Type:        TypeLibrary,
		Description: "Lightweight C standard library for Alpine Linux",
		Path:        "/lib/ld-musl-x86_64.so.1",
		Tags:        []string{"library", "libc", "musl", "alpine"},
		Metadata: map[string]string{
			"arch":       "x86_64",
			"abi":        "musl",
			"importance": "critical",
		},
	},
	"kernel": {
		Name:        "kernel",
		Version:     "5.15.0",
		Type:        TypeKernel,
		Description: "Linux kernel for ROCK-OS",
		Path:        "/boot/vmlinuz",
		Tags:        []string{"kernel", "boot", "linux"},
		Metadata: map[string]string{
			"config":     "rock-os-defconfig",
			"modules":    "true",
			"compression": "gzip",
			"importance": "critical",
		},
	},
}

func init() {
	// Set up registry directory
	registryDir = os.Getenv("ROCK_REGISTRY_DIR")
	if registryDir == "" {
		homeDir, _ := os.UserHomeDir()
		registryDir = filepath.Join(homeDir, DefaultRegistryDir)
	}
	registryPath = filepath.Join(registryDir, RegistryFile)

	// Check for JSON output
	if os.Getenv("ROCK_OUTPUT") == "json" {
		jsonOutput = true
	}

	// Check for verbose mode
	if os.Getenv("ROCK_VERBOSE") == "true" {
		verboseMode = true
	}
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	// Initialize registry
	if err := initializeRegistry(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing registry: %v\n", err)
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "list":
		cmdList()

	case "add":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: add requires component name\n")
			showUsage()
			os.Exit(1)
		}
		cmdAdd(os.Args[2])

	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: get requires component name\n")
			showUsage()
			os.Exit(1)
		}
		cmdGet(os.Args[2])

	case "search":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: search requires pattern\n")
			showUsage()
			os.Exit(1)
		}
		cmdSearch(os.Args[2])

	case "remove":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: remove requires component name\n")
			showUsage()
			os.Exit(1)
		}
		cmdRemove(os.Args[2])

	case "update":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: update requires component name\n")
			showUsage()
			os.Exit(1)
		}
		cmdUpdate(os.Args[2])

	case "deps":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: deps requires component name\n")
			showUsage()
			os.Exit(1)
		}
		cmdDeps(os.Args[2])

	case "export":
		if len(os.Args) < 3 {
			cmdExport("")
		} else {
			cmdExport(os.Args[2])
		}

	case "import":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: import requires file path\n")
			showUsage()
			os.Exit(1)
		}
		cmdImport(os.Args[2])

	case "init":
		cmdInit()

	case "stats":
		cmdStats()

	case "version":
		fmt.Printf("rock-registry version %s\n", Version)
		fmt.Printf("  Build time: %s\n", BuildTime)
		fmt.Printf("  Git commit: %s\n", GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-registry - Component registry for ROCK-OS

Usage:
  rock-registry <command> [arguments]

Commands:
  list                List all registered components
  add <component>     Register a new component
  get <component>     Get component information
  search <pattern>    Search for components
  remove <component>  Remove a component
  update <component>  Update component information
  deps <component>    Show component dependencies
  export [file]       Export registry to file
  import <file>       Import registry from file
  init                Initialize with built-in components
  stats               Show registry statistics
  version             Show version information

Component Types:
  binary   - Executable binaries
  library  - Shared libraries
  config   - Configuration files
  kernel   - Kernel images
  initrd   - Initramfs images
  tool     - Build tools
  runtime  - Runtime environments

Environment Variables:
  ROCK_REGISTRY_DIR   Registry directory (default: ~/.rock-registry)
  ROCK_OUTPUT         Output format (json/text)
  ROCK_VERBOSE        Enable verbose output

Examples:
  rock-registry list
  rock-registry add rock-init --type binary --path /sbin/init
  rock-registry get volcano-agent
  rock-registry search "volcano*"
  rock-registry deps rock-manager`)
}

func initializeRegistry() error {
	// Create registry directory if it doesn't exist
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return fmt.Errorf("failed to create registry directory: %v", err)
	}

	// Create components directory
	componentsDir := filepath.Join(registryDir, ComponentsDir)
	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create components directory: %v", err)
	}

	// Check if registry exists, if not create empty one
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		registry := &Registry{
			Version:    "1.0",
			Components: make(map[string]*Component),
			Updated:    time.Now(),
		}
		return saveRegistry(registry)
	}

	return nil
}

func cmdList() {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	if len(registry.Components) == 0 {
		if !jsonOutput {
			fmt.Println("No components registered")
			fmt.Println("Run 'rock-registry init' to initialize with built-in components")
		} else {
			fmt.Println("[]")
		}
		return
	}

	// Sort components by name
	var components []*Component
	for _, comp := range registry.Components {
		components = append(components, comp)
	}
	sort.Slice(components, func(i, j int) bool {
		return components[i].Name < components[j].Name
	})

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(components)
	} else {
		fmt.Printf("Registered Components (%d):\n", len(components))
		fmt.Println("=" + strings.Repeat("=", 50))

		// Group by type
		byType := make(map[string][]*Component)
		for _, comp := range components {
			byType[comp.Type] = append(byType[comp.Type], comp)
		}

		// Display by type
		typeOrder := []string{TypeBinary, TypeLibrary, TypeKernel, TypeConfig, TypeTool, TypeRuntime, TypeInitrd}
		for _, t := range typeOrder {
			if comps, ok := byType[t]; ok && len(comps) > 0 {
				fmt.Printf("\n📦 %s:\n", strings.Title(t))
				for _, comp := range comps {
					fmt.Printf("  • %s (v%s)", comp.Name, comp.Version)
					if comp.Description != "" {
						fmt.Printf(" - %s", comp.Description)
					}
					fmt.Println()
					if verboseMode && comp.Path != "" {
						fmt.Printf("    Path: %s\n", comp.Path)
					}
				}
			}
		}
	}
}

func cmdAdd(name string) {
	// Parse additional arguments
	component := &Component{
		Name:       name,
		Version:    "1.0.0",
		Type:       TypeBinary,
		Registered: time.Now(),
		Updated:    time.Now(),
		Tags:       []string{},
		Metadata:   make(map[string]string),
	}

	// Parse flags (simplified)
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--type=") {
			component.Type = strings.TrimPrefix(arg, "--type=")
		} else if strings.HasPrefix(arg, "--path=") {
			component.Path = strings.TrimPrefix(arg, "--path=")
		} else if strings.HasPrefix(arg, "--version=") {
			component.Version = strings.TrimPrefix(arg, "--version=")
		} else if strings.HasPrefix(arg, "--desc=") {
			component.Description = strings.TrimPrefix(arg, "--desc=")
		} else if strings.HasPrefix(arg, "--deps=") {
			deps := strings.TrimPrefix(arg, "--deps=")
			component.Dependencies = strings.Split(deps, ",")
		} else if strings.HasPrefix(arg, "--tags=") {
			tags := strings.TrimPrefix(arg, "--tags=")
			component.Tags = strings.Split(tags, ",")
		}
	}

	// Load registry
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	// Check if component already exists
	if _, exists := registry.Components[name]; exists {
		fmt.Fprintf(os.Stderr, "Error: component '%s' already exists\n", name)
		os.Exit(1)
	}

	// Add component
	registry.Components[name] = component
	registry.Updated = time.Now()

	// Save registry
	if err := saveRegistry(registry); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving registry: %v\n", err)
		os.Exit(1)
	}

	// Save component details
	componentPath := filepath.Join(registryDir, ComponentsDir, name+".json")
	if err := saveComponent(component, componentPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save component details: %v\n", err)
	}

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(component)
	} else {
		fmt.Printf("✅ Registered component '%s'\n", name)
		fmt.Printf("   Type: %s\n", component.Type)
		fmt.Printf("   Version: %s\n", component.Version)
		if component.Path != "" {
			fmt.Printf("   Path: %s\n", component.Path)
		}
	}
}

func cmdGet(name string) {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	component, exists := registry.Components[name]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: component '%s' not found\n", name)
		os.Exit(1)
	}

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(component)
	} else {
		fmt.Printf("Component: %s\n", component.Name)
		fmt.Printf("=" + strings.Repeat("=", 40) + "\n")
		fmt.Printf("Version:     %s\n", component.Version)
		fmt.Printf("Type:        %s\n", component.Type)

		if component.Description != "" {
			fmt.Printf("Description: %s\n", component.Description)
		}

		if component.Path != "" {
			fmt.Printf("Path:        %s\n", component.Path)
		}

		if component.URL != "" {
			fmt.Printf("URL:         %s\n", component.URL)
		}

		if len(component.Dependencies) > 0 {
			fmt.Printf("Dependencies:\n")
			for _, dep := range component.Dependencies {
				fmt.Printf("  • %s\n", dep)
			}
		}

		if len(component.Tags) > 0 {
			fmt.Printf("Tags:        %s\n", strings.Join(component.Tags, ", "))
		}

		if len(component.Metadata) > 0 {
			fmt.Printf("Metadata:\n")
			keys := make([]string, 0, len(component.Metadata))
			for k := range component.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %s: %s\n", k, component.Metadata[k])
			}
		}

		fmt.Printf("Registered:  %s\n", component.Registered.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated:     %s\n", component.Updated.Format("2006-01-02 15:04:05"))
	}
}

func cmdSearch(pattern string) {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	// Compile regex pattern
	re, err := regexp.Compile(pattern)
	if err != nil {
		// If not valid regex, treat as literal substring
		pattern = regexp.QuoteMeta(pattern)
		re = regexp.MustCompile(pattern)
	}

	var results []*SearchResult

	for _, component := range registry.Components {
		matches := []string{}
		score := 0

		// Check name
		if re.MatchString(component.Name) {
			matches = append(matches, "name")
			score += 10
		}

		// Check description
		if re.MatchString(component.Description) {
			matches = append(matches, "description")
			score += 5
		}

		// Check type
		if re.MatchString(component.Type) {
			matches = append(matches, "type")
			score += 3
		}

		// Check tags
		for _, tag := range component.Tags {
			if re.MatchString(tag) {
				matches = append(matches, fmt.Sprintf("tag:%s", tag))
				score += 2
			}
		}

		// Check metadata
		for key, value := range component.Metadata {
			if re.MatchString(value) {
				matches = append(matches, fmt.Sprintf("metadata:%s", key))
				score += 1
			}
		}

		if score > 0 {
			results = append(results, &SearchResult{
				Component: component,
				Score:     score,
				Matches:   matches,
			})
		}
	}

	// Sort by score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(results)
	} else {
		if len(results) == 0 {
			fmt.Printf("No components matching '%s'\n", pattern)
		} else {
			fmt.Printf("Found %d components matching '%s':\n", len(results), pattern)
			fmt.Println("=" + strings.Repeat("=", 50))

			for _, result := range results {
				comp := result.Component
				fmt.Printf("\n📦 %s (v%s) - %s\n", comp.Name, comp.Version, comp.Type)

				if comp.Description != "" {
					fmt.Printf("   %s\n", comp.Description)
				}

				if verboseMode {
					fmt.Printf("   Matches: %s\n", strings.Join(result.Matches, ", "))
					fmt.Printf("   Score: %d\n", result.Score)
				}
			}
		}
	}
}

func cmdRemove(name string) {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	if _, exists := registry.Components[name]; !exists {
		fmt.Fprintf(os.Stderr, "Error: component '%s' not found\n", name)
		os.Exit(1)
	}

	// Remove component
	delete(registry.Components, name)
	registry.Updated = time.Now()

	// Save registry
	if err := saveRegistry(registry); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving registry: %v\n", err)
		os.Exit(1)
	}

	// Remove component file
	componentPath := filepath.Join(registryDir, ComponentsDir, name+".json")
	os.Remove(componentPath)

	if jsonOutput {
		fmt.Printf(`{"removed":"%s"}`, name)
	} else {
		fmt.Printf("✅ Removed component '%s'\n", name)
	}
}

func cmdUpdate(name string) {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	component, exists := registry.Components[name]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: component '%s' not found\n", name)
		os.Exit(1)
	}

	// Parse update flags
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--version=") {
			component.Version = strings.TrimPrefix(arg, "--version=")
		} else if strings.HasPrefix(arg, "--path=") {
			component.Path = strings.TrimPrefix(arg, "--path=")
		} else if strings.HasPrefix(arg, "--desc=") {
			component.Description = strings.TrimPrefix(arg, "--desc=")
		}
	}

	component.Updated = time.Now()
	registry.Updated = time.Now()

	// Save registry
	if err := saveRegistry(registry); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving registry: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(component)
	} else {
		fmt.Printf("✅ Updated component '%s'\n", name)
	}
}

func cmdDeps(name string) {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	component, exists := registry.Components[name]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: component '%s' not found\n", name)
		os.Exit(1)
	}

	// Build dependency tree
	depTree := buildDependencyTree(registry, name, 0, make(map[string]bool))

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(depTree)
	} else {
		fmt.Printf("Dependencies for %s:\n", name)
		fmt.Println("=" + strings.Repeat("=", 40))

		if len(component.Dependencies) == 0 {
			fmt.Println("No dependencies")
		} else {
			printDependencyTree(registry, name, 0, make(map[string]bool))
		}
	}
}

func cmdExport(filename string) {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	var output []byte
	if jsonOutput || filename == "" {
		output, err = json.MarshalIndent(registry, "", "  ")
	} else {
		output, err = json.MarshalIndent(registry, "", "  ")
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling registry: %v\n", err)
		os.Exit(1)
	}

	if filename == "" {
		fmt.Println(string(output))
	} else {
		if err := os.WriteFile(filename, output, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		if !jsonOutput {
			fmt.Printf("✅ Exported registry to %s\n", filename)
		}
	}
}

func cmdImport(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var importedRegistry Registry
	if err := json.Unmarshal(data, &importedRegistry); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing registry: %v\n", err)
		os.Exit(1)
	}

	// Load current registry
	registry, err := loadRegistry()
	if err != nil {
		registry = &Registry{
			Version:    "1.0",
			Components: make(map[string]*Component),
			Updated:    time.Now(),
		}
	}

	// Merge components
	imported := 0
	updated := 0
	for name, comp := range importedRegistry.Components {
		if _, exists := registry.Components[name]; exists {
			updated++
		} else {
			imported++
		}
		registry.Components[name] = comp
	}

	registry.Updated = time.Now()

	// Save registry
	if err := saveRegistry(registry); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving registry: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		result := map[string]int{
			"imported": imported,
			"updated":  updated,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("✅ Import complete\n")
		fmt.Printf("   Imported: %d new components\n", imported)
		fmt.Printf("   Updated: %d existing components\n", updated)
	}
}

func cmdInit() {
	registry, err := loadRegistry()
	if err != nil {
		registry = &Registry{
			Version:    "1.0",
			Components: make(map[string]*Component),
			Updated:    time.Now(),
		}
	}

	// Add built-in components
	added := 0
	for name, comp := range builtInComponents {
		if _, exists := registry.Components[name]; !exists {
			comp.Registered = time.Now()
			comp.Updated = time.Now()
			registry.Components[name] = comp
			added++
		}
	}

	registry.Updated = time.Now()

	// Save registry
	if err := saveRegistry(registry); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving registry: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(registry)
	} else {
		fmt.Printf("✅ Initialized registry with %d built-in components\n", added)
		fmt.Println("\nAdded components:")
		for name := range builtInComponents {
			fmt.Printf("  • %s\n", name)
		}
	}
}

func cmdStats() {
	registry, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	// Calculate statistics
	stats := map[string]interface{}{
		"total_components": len(registry.Components),
		"registry_version": registry.Version,
		"last_updated":     registry.Updated,
	}

	// Count by type
	byType := make(map[string]int)
	totalDeps := 0
	for _, comp := range registry.Components {
		byType[comp.Type]++
		totalDeps += len(comp.Dependencies)
	}
	stats["by_type"] = byType
	stats["total_dependencies"] = totalDeps

	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(stats)
	} else {
		fmt.Println("Registry Statistics:")
		fmt.Println("=" + strings.Repeat("=", 40))
		fmt.Printf("Total components: %d\n", len(registry.Components))
		fmt.Printf("Registry version: %s\n", registry.Version)
		fmt.Printf("Last updated:     %s\n", registry.Updated.Format("2006-01-02 15:04:05"))
		fmt.Println("\nComponents by type:")

		typeOrder := []string{TypeBinary, TypeLibrary, TypeKernel, TypeConfig, TypeTool, TypeRuntime, TypeInitrd}
		for _, t := range typeOrder {
			if count, ok := byType[t]; ok && count > 0 {
				fmt.Printf("  • %s: %d\n", strings.Title(t), count)
			}
		}

		fmt.Printf("\nTotal dependencies: %d\n", totalDeps)
		fmt.Printf("Registry location:  %s\n", registryDir)
	}
}

// Helper functions

func loadRegistry() (*Registry, error) {
	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty registry if file doesn't exist
			return &Registry{
				Version:    "1.0",
				Components: make(map[string]*Component),
				Updated:    time.Now(),
			}, nil
		}
		return nil, err
	}

	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, err
	}

	if registry.Components == nil {
		registry.Components = make(map[string]*Component)
	}

	return &registry, nil
}

func saveRegistry(registry *Registry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(registryPath, data, 0644)
}

func saveComponent(component *Component, path string) error {
	data, err := json.MarshalIndent(component, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func buildDependencyTree(registry *Registry, name string, depth int, visited map[string]bool) map[string]interface{} {
	if visited[name] {
		return map[string]interface{}{
			"name":     name,
			"circular": true,
		}
	}
	visited[name] = true

	component, exists := registry.Components[name]
	if !exists {
		return map[string]interface{}{
			"name":     name,
			"missing":  true,
		}
	}

	result := map[string]interface{}{
		"name":    name,
		"version": component.Version,
		"type":    component.Type,
	}

	if len(component.Dependencies) > 0 {
		deps := []map[string]interface{}{}
		for _, depName := range component.Dependencies {
			deps = append(deps, buildDependencyTree(registry, depName, depth+1, visited))
		}
		result["dependencies"] = deps
	}

	return result
}

func printDependencyTree(registry *Registry, name string, depth int, visited map[string]bool) {
	indent := strings.Repeat("  ", depth)

	if visited[name] {
		fmt.Printf("%s• %s (circular reference)\n", indent, name)
		return
	}
	visited[name] = true

	component, exists := registry.Components[name]
	if !exists {
		fmt.Printf("%s• %s (not found)\n", indent, name)
		return
	}

	if depth == 0 {
		fmt.Printf("%s• %s (v%s)\n", indent, name, component.Version)
	} else {
		fmt.Printf("%s└─ %s (v%s)\n", indent, name, component.Version)
	}

	for _, depName := range component.Dependencies {
		printDependencyTree(registry, depName, depth+1, visited)
	}
}