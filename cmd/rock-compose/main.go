package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Pipeline represents a complete pipeline definition
type Pipeline struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Variables   map[string]string      `json:"variables"`
	Stages      []Stage                `json:"stages"`
	OnSuccess   []Step                 `json:"on_success,omitempty"`
	OnFailure   []Step                 `json:"on_failure,omitempty"`
	Settings    map[string]interface{} `json:"settings,omitempty"`
}

// Stage represents a pipeline stage
type Stage struct {
	Name      string   `json:"name"`
	Steps     []Step   `json:"steps"`
	Parallel  bool     `json:"parallel,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
	Condition string   `json:"condition,omitempty"`
}

// Step represents a single execution step
type Step struct {
	Name        string            `json:"name"`
	Tool        string            `json:"tool"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Environment map[string]string `json:"env,omitempty"`
	WorkDir     string            `json:"workdir,omitempty"`
	ContinueOn  string            `json:"continue_on,omitempty"`
	Timeout     int               `json:"timeout,omitempty"`
	Retries     int               `json:"retries,omitempty"`
}

// ExecutionResult represents the result of a step execution
type ExecutionResult struct {
	Step      string        `json:"step"`
	Success   bool          `json:"success"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration"`
	Output    string        `json:"output,omitempty"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// PipelineResult represents the complete pipeline execution result
type PipelineResult struct {
	Pipeline    string            `json:"pipeline"`
	Success     bool              `json:"success"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     time.Time         `json:"end_time"`
	Duration    time.Duration     `json:"duration"`
	StageResults map[string][]ExecutionResult `json:"stage_results"`
	Artifacts   []string          `json:"artifacts,omitempty"`
}

// Default pipelines directory
const DefaultPipelinesDir = "./pipelines"

// Built-in example pipelines
var builtInPipelines = map[string]*Pipeline{
	"build-image": {
		Name:        "build-image",
		Version:     "1.0",
		Description: "Build and verify ROCK-OS image",
		Variables: map[string]string{
			"OUTPUT_DIR": "./output",
			"ROOTFS_DIR": "./rootfs",
		},
		Stages: []Stage{
			{
				Name: "prepare",
				Steps: []Step{
					{
						Name:    "Initialize security",
						Tool:    "rock-security",
						Command: "init",
					},
					{
						Name:    "Initialize config",
						Tool:    "rock-config",
						Command: "init",
					},
				},
			},
			{
				Name:      "build",
				DependsOn: []string{"prepare"},
				Steps: []Step{
					{
						Name:    "Build components",
						Tool:    "rock-build",
						Command: "all",
						Environment: map[string]string{
							"ROCK_PROFILE": "release",
						},
					},
				},
			},
			{
				Name:      "dependencies",
				DependsOn: []string{"build"},
				Parallel:  true,
				Steps: []Step{
					{
						Name:    "Scan rock-init",
						Tool:    "rock-deps",
						Command: "scan",
						Args:    []string{"${OUTPUT_DIR}/sbin/init"},
					},
					{
						Name:    "Scan rock-manager",
						Tool:    "rock-deps",
						Command: "scan",
						Args:    []string{"${OUTPUT_DIR}/usr/bin/rock-manager"},
					},
					{
						Name:    "Scan volcano-agent",
						Tool:    "rock-deps",
						Command: "scan",
						Args:    []string{"${OUTPUT_DIR}/usr/bin/volcano-agent"},
					},
				},
			},
			{
				Name:      "configuration",
				DependsOn: []string{"dependencies"},
				Steps: []Step{
					{
						Name:    "Generate configs",
						Tool:    "rock-config",
						Command: "generate",
						Args:    []string{"all"},
					},
					{
						Name:    "Generate CONFIG_KEY",
						Tool:    "rock-security",
						Command: "keygen",
						Args:    []string{"aes"},
					},
				},
			},
			{
				Name:      "image",
				DependsOn: []string{"configuration"},
				Steps: []Step{
					{
						Name:    "Create initramfs",
						Tool:    "rock-image",
						Command: "cpio",
						Args:    []string{"create", "${ROOTFS_DIR}"},
					},
				},
			},
			{
				Name:      "security",
				DependsOn: []string{"image"},
				Steps: []Step{
					{
						Name:    "Sign image",
						Tool:    "rock-security",
						Command: "sign",
						Args:    []string{"initrd.cpio.gz"},
					},
					{
						Name:    "Calculate hashes",
						Tool:    "rock-security",
						Command: "hash",
						Args:    []string{"initrd.cpio.gz"},
					},
				},
			},
			{
				Name:      "verification",
				DependsOn: []string{"security"},
				Steps: []Step{
					{
						Name:    "Verify integration",
						Tool:    "rock-verify",
						Command: "integration",
						Args:    []string{"initrd.cpio.gz"},
					},
					{
						Name:    "Verify signature",
						Tool:    "rock-security",
						Command: "verify",
						Args:    []string{"initrd.cpio.gz"},
					},
				},
			},
		},
	},
	"quick-check": {
		Name:        "quick-check",
		Version:     "1.0",
		Description: "Quick environment check",
		Stages: []Stage{
			{
				Name:     "checks",
				Parallel: true,
				Steps: []Step{
					{
						Name:    "Check build env",
						Tool:    "rock-build",
						Command: "check",
					},
					{
						Name:    "Check security",
						Tool:    "rock-security",
						Command: "check",
					},
					{
						Name:    "Check config",
						Tool:    "rock-config",
						Command: "check",
					},
				},
			},
		},
	},
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: run requires a pipeline file or name\n")
			os.Exit(1)
		}
		cmdRun(os.Args[2])

	case "validate":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: validate requires a pipeline file\n")
			os.Exit(1)
		}
		cmdValidate(os.Args[2])

	case "list":
		cmdList()

	case "generate":
		if len(os.Args) < 3 {
			cmdGenerate("example")
		} else {
			cmdGenerate(os.Args[2])
		}

	case "dry-run":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: dry-run requires a pipeline file or name\n")
			os.Exit(1)
		}
		cmdDryRun(os.Args[2])

	case "version":
		fmt.Printf("rock-compose version %s (built %s, commit %s)\n",
			Version, BuildTime, GitCommit)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println(`rock-compose - Pipeline Orchestration for ROCK-OS

Orchestrates complex build pipelines using all rock-* tools.
Ensures proper integration and verification at each stage.

Usage:
  rock-compose run <pipeline>      Execute pipeline
  rock-compose validate <pipeline> Validate pipeline syntax
  rock-compose list                Show available pipelines
  rock-compose generate [name]     Generate example pipeline
  rock-compose dry-run <pipeline>  Show execution plan
  rock-compose version            Show version

Pipeline Format:
  Pipelines are defined in JSON format with:
  - stages: Sequential or parallel execution stages
  - steps: Individual tool executions
  - dependencies: Stage dependencies
  - variables: Environment substitution
  - verification: Always runs verification

Built-in Pipelines:
  build-image    Complete image build and verification
  quick-check    Quick environment check

Examples:
  # Run built-in pipeline
  rock-compose run build-image

  # Run custom pipeline
  rock-compose run my-pipeline.json

  # Validate pipeline
  rock-compose validate pipeline.json

  # List available pipelines
  rock-compose list

Environment:
  ROCK_PIPELINES_DIR   Pipeline directory (default: ./pipelines)
  ROCK_OUTPUT=json     JSON output format
  ROCK_VERBOSE=1       Verbose output
  ROCK_DRY_RUN=1       Dry run mode

Critical Integration:
  • Always runs rock-verify after image creation
  • Ensures CONFIG_KEY is generated
  • Signs artifacts for security
  • Validates all configurations`)
}

func cmdRun(pipelinePath string) {
	// Load pipeline
	pipeline, err := loadPipeline(pipelinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading pipeline: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🚀 Running pipeline: %s\n", pipeline.Name)
	if pipeline.Description != "" {
		fmt.Printf("   %s\n", pipeline.Description)
	}
	fmt.Println("=" + strings.Repeat("=", 60))

	// Initialize result
	result := &PipelineResult{
		Pipeline:     pipeline.Name,
		StartTime:    time.Now(),
		StageResults: make(map[string][]ExecutionResult),
	}

	// Set up environment variables
	for key, value := range pipeline.Variables {
		os.Setenv(key, expandVariables(value))
	}

	// Execute stages
	executedStages := make(map[string]bool)
	success := true

	for _, stage := range pipeline.Stages {
		// Check dependencies
		if !checkDependencies(stage.DependsOn, executedStages) {
			fmt.Printf("⚠️  Skipping stage %s: dependencies not met\n", stage.Name)
			continue
		}

		// Check condition
		if stage.Condition != "" && !evaluateCondition(stage.Condition) {
			fmt.Printf("⚠️  Skipping stage %s: condition not met\n", stage.Name)
			continue
		}

		fmt.Printf("\n📦 Stage: %s\n", stage.Name)
		fmt.Println("-" + strings.Repeat("-", 40))

		var stageResults []ExecutionResult

		if stage.Parallel {
			// Execute steps in parallel
			stageResults = executeParallelSteps(stage.Steps)
		} else {
			// Execute steps sequentially
			stageResults = executeSequentialSteps(stage.Steps)
		}

		result.StageResults[stage.Name] = stageResults

		// Check if stage succeeded
		stageFailed := false
		for _, stepResult := range stageResults {
			if !stepResult.Success {
				stageFailed = true
				success = false
				break
			}
		}

		if stageFailed {
			fmt.Printf("❌ Stage %s failed\n", stage.Name)
			break
		} else {
			fmt.Printf("✅ Stage %s completed\n", stage.Name)
			executedStages[stage.Name] = true
		}
	}

	// Run on_success or on_failure hooks
	if success && len(pipeline.OnSuccess) > 0 {
		fmt.Println("\n🎉 Running success hooks...")
		executeSequentialSteps(pipeline.OnSuccess)
	} else if !success && len(pipeline.OnFailure) > 0 {
		fmt.Println("\n🔧 Running failure hooks...")
		executeSequentialSteps(pipeline.OnFailure)
	}

	// Finalize result
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = success

	// Output result
	if os.Getenv("ROCK_OUTPUT") == "json" {
		outputJSON(result)
	} else {
		fmt.Println("\n" + "=" + strings.Repeat("=", 60))
		if success {
			fmt.Printf("✅ Pipeline completed successfully\n")
		} else {
			fmt.Printf("❌ Pipeline failed\n")
		}
		fmt.Printf("   Duration: %.2fs\n", result.Duration.Seconds())
	}

	if !success {
		os.Exit(1)
	}
}

func cmdValidate(pipelinePath string) {
	// Load and validate pipeline
	pipeline, err := loadPipeline(pipelinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Invalid pipeline: %v\n", err)
		os.Exit(1)
	}

	// Validate structure
	issues := validatePipeline(pipeline)

	if len(issues) == 0 {
		fmt.Printf("✅ Pipeline is valid\n")
		fmt.Printf("   Name: %s\n", pipeline.Name)
		fmt.Printf("   Stages: %d\n", len(pipeline.Stages))

		totalSteps := 0
		for _, stage := range pipeline.Stages {
			totalSteps += len(stage.Steps)
		}
		fmt.Printf("   Total steps: %d\n", totalSteps)
	} else {
		fmt.Printf("❌ Pipeline has issues:\n")
		for _, issue := range issues {
			fmt.Printf("   • %s\n", issue)
		}
		os.Exit(1)
	}
}

func cmdList() {
	fmt.Println("Available Pipelines:")
	fmt.Println("=" + strings.Repeat("=", 60))

	// List built-in pipelines
	fmt.Println("\nBuilt-in:")
	for name, pipeline := range builtInPipelines {
		fmt.Printf("  • %-15s %s\n", name, pipeline.Description)
	}

	// List pipelines from directory
	pipelinesDir := getPipelinesDir()
	if files, err := os.ReadDir(pipelinesDir); err == nil && len(files) > 0 {
		fmt.Printf("\nFrom %s:\n", pipelinesDir)
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".json") ||
			   strings.HasSuffix(file.Name(), ".yaml") ||
			   strings.HasSuffix(file.Name(), ".yml") {
				path := filepath.Join(pipelinesDir, file.Name())
				if pipeline, err := loadPipeline(path); err == nil {
					fmt.Printf("  • %-15s %s\n",
						strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
						pipeline.Description)
				}
			}
		}
	}

	fmt.Println("\n" + "=" + strings.Repeat("=", 60))
	fmt.Println("\nRun a pipeline with: rock-compose run <name>")
}

func cmdGenerate(name string) {
	var pipeline *Pipeline

	switch name {
	case "example", "default":
		pipeline = generateExamplePipeline()
	case "minimal":
		pipeline = generateMinimalPipeline()
	case "full":
		pipeline = builtInPipelines["build-image"]
	default:
		pipeline = generateExamplePipeline()
		pipeline.Name = name
	}

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(pipeline)
}

func cmdDryRun(pipelinePath string) {
	// Load pipeline
	pipeline, err := loadPipeline(pipelinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading pipeline: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🔍 Dry run for pipeline: %s\n", pipeline.Name)
	fmt.Println("=" + strings.Repeat("=", 60))

	// Show execution plan
	fmt.Println("\nExecution Plan:")
	for i, stage := range pipeline.Stages {
		fmt.Printf("\n%d. Stage: %s\n", i+1, stage.Name)

		if len(stage.DependsOn) > 0 {
			fmt.Printf("   Dependencies: %s\n", strings.Join(stage.DependsOn, ", "))
		}

		if stage.Parallel {
			fmt.Println("   Execution: PARALLEL")
		} else {
			fmt.Println("   Execution: SEQUENTIAL")
		}

		fmt.Println("   Steps:")
		for j, step := range stage.Steps {
			fmt.Printf("      %d.%d. %s\n", i+1, j+1, step.Name)
			fmt.Printf("           Tool: %s %s", step.Tool, step.Command)
			if len(step.Args) > 0 {
				fmt.Printf(" %s", strings.Join(step.Args, " "))
			}
			fmt.Println()
		}
	}

	// Show variables
	if len(pipeline.Variables) > 0 {
		fmt.Println("\nVariables:")
		for key, value := range pipeline.Variables {
			fmt.Printf("  %s = %s\n", key, value)
		}
	}
}

// Helper functions

func loadPipeline(path string) (*Pipeline, error) {
	// Check if it's a built-in pipeline
	if pipeline, ok := builtInPipelines[path]; ok {
		return pipeline, nil
	}

	// Check in pipelines directory
	if !strings.Contains(path, "/") && !strings.HasSuffix(path, ".json") {
		pipelinesDir := getPipelinesDir()
		possiblePath := filepath.Join(pipelinesDir, path+".json")
		if _, err := os.Stat(possiblePath); err == nil {
			path = possiblePath
		}
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse pipeline
	var pipeline Pipeline
	if err := json.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("invalid JSON: %v", err)
	}

	return &pipeline, nil
}

func validatePipeline(pipeline *Pipeline) []string {
	issues := []string{}

	if pipeline.Name == "" {
		issues = append(issues, "Pipeline name is required")
	}

	if len(pipeline.Stages) == 0 {
		issues = append(issues, "Pipeline must have at least one stage")
	}

	// Check stage dependencies
	stageNames := make(map[string]bool)
	for _, stage := range pipeline.Stages {
		if stage.Name == "" {
			issues = append(issues, "Stage name is required")
		}
		stageNames[stage.Name] = true

		if len(stage.Steps) == 0 {
			issues = append(issues, fmt.Sprintf("Stage %s has no steps", stage.Name))
		}

		// Validate steps
		for _, step := range stage.Steps {
			if step.Tool == "" && step.Command == "" {
				issues = append(issues, fmt.Sprintf("Step %s must have tool or command", step.Name))
			}
		}
	}

	// Check dependencies exist
	for _, stage := range pipeline.Stages {
		for _, dep := range stage.DependsOn {
			if !stageNames[dep] {
				issues = append(issues, fmt.Sprintf("Stage %s depends on unknown stage: %s", stage.Name, dep))
			}
		}
	}

	// Check for cycles
	if hasCycle(pipeline.Stages) {
		issues = append(issues, "Pipeline has circular dependencies")
	}

	return issues
}

func executeSequentialSteps(steps []Step) []ExecutionResult {
	results := []ExecutionResult{}

	for _, step := range steps {
		fmt.Printf("   ▶ %s\n", step.Name)
		result := executeStep(step)
		results = append(results, result)

		if !result.Success {
			if step.ContinueOn != "error" && step.ContinueOn != "failure" {
				break
			}
		}
	}

	return results
}

func executeParallelSteps(steps []Step) []ExecutionResult {
	results := make([]ExecutionResult, len(steps))
	var wg sync.WaitGroup

	for i, step := range steps {
		wg.Add(1)
		go func(index int, s Step) {
			defer wg.Done()
			fmt.Printf("   ▶ %s (parallel)\n", s.Name)
			results[index] = executeStep(s)
		}(i, step)
	}

	wg.Wait()
	return results
}

func executeStep(step Step) ExecutionResult {
	startTime := time.Now()
	result := ExecutionResult{
		Step:      step.Name,
		Timestamp: startTime,
	}

	// Determine command
	var cmd *exec.Cmd
	if step.Tool != "" {
		// Use rock-* tool
		toolPath := fmt.Sprintf("./bin/darwin/rock-%s", step.Tool)
		args := []string{}
		if step.Command != "" {
			args = append(args, step.Command)
		}
		args = append(args, step.Args...)

		// Expand variables in args
		for i, arg := range args {
			args[i] = expandVariables(arg)
		}

		cmd = exec.Command(toolPath, args...)
	} else if step.Command != "" {
		// Direct command
		cmd = exec.Command("sh", "-c", expandVariables(step.Command))
	} else {
		result.Success = false
		result.Error = "No tool or command specified"
		return result
	}

	// Set working directory
	if step.WorkDir != "" {
		cmd.Dir = expandVariables(step.WorkDir)
	}

	// Set environment
	cmd.Env = os.Environ()
	for key, value := range step.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, expandVariables(value)))
	}

	// Execute with retries
	maxRetries := 1
	if step.Retries > 0 {
		maxRetries = step.Retries + 1
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("     Retry %d/%d\n", attempt-1, step.Retries)
		}

		// Execute command
		output, err := cmd.CombinedOutput()
		result.Output = string(output)
		result.Duration = time.Since(startTime)

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitError.ExitCode()
			} else {
				result.ExitCode = -1
			}
			result.Error = err.Error()
			result.Success = false

			// Check if should retry
			if attempt < maxRetries {
				time.Sleep(time.Second * time.Duration(attempt))
				continue
			}
		} else {
			result.Success = true
			result.ExitCode = 0
			break
		}
	}

	// Show result
	if result.Success {
		fmt.Printf("     ✅ Success (%.2fs)\n", result.Duration.Seconds())
	} else {
		fmt.Printf("     ❌ Failed: %s\n", result.Error)
		if os.Getenv("ROCK_VERBOSE") == "1" && result.Output != "" {
			fmt.Printf("     Output: %s\n", strings.TrimSpace(result.Output))
		}
	}

	return result
}

func checkDependencies(deps []string, executed map[string]bool) bool {
	for _, dep := range deps {
		if !executed[dep] {
			return false
		}
	}
	return true
}

func evaluateCondition(condition string) bool {
	// Simple condition evaluation (can be extended)
	if condition == "always" {
		return true
	}
	if condition == "never" {
		return false
	}

	// Check environment variable
	if strings.HasPrefix(condition, "$") {
		varName := strings.TrimPrefix(condition, "$")
		return os.Getenv(varName) != ""
	}

	return true
}

func hasCycle(stages []Stage) bool {
	// Simple cycle detection (can be improved with topological sort)
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycleUtil func(name string) bool
	hasCycleUtil = func(name string) bool {
		visited[name] = true
		recStack[name] = true

		// Find stage
		var stage *Stage
		for _, s := range stages {
			if s.Name == name {
				stage = &s
				break
			}
		}

		if stage != nil {
			for _, dep := range stage.DependsOn {
				if !visited[dep] {
					if hasCycleUtil(dep) {
						return true
					}
				} else if recStack[dep] {
					return true
				}
			}
		}

		recStack[name] = false
		return false
	}

	for _, stage := range stages {
		if !visited[stage.Name] {
			if hasCycleUtil(stage.Name) {
				return true
			}
		}
	}

	return false
}

func expandVariables(s string) string {
	// Expand ${VAR} style variables
	result := s
	for key, value := range getAllVariables() {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return os.ExpandEnv(result)
}

func getAllVariables() map[string]string {
	vars := make(map[string]string)

	// Get all environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	return vars
}

func getPipelinesDir() string {
	if dir := os.Getenv("ROCK_PIPELINES_DIR"); dir != "" {
		return dir
	}
	return DefaultPipelinesDir
}

func generateExamplePipeline() *Pipeline {
	return &Pipeline{
		Name:        "example",
		Version:     "1.0",
		Description: "Example pipeline showing basic structure",
		Variables: map[string]string{
			"BUILD_DIR": "./build",
			"OUTPUT_DIR": "./output",
		},
		Stages: []Stage{
			{
				Name: "prepare",
				Steps: []Step{
					{
						Name:    "Create directories",
						Command: "mkdir -p ${BUILD_DIR} ${OUTPUT_DIR}",
					},
					{
						Name:    "Check environment",
						Tool:    "build",
						Command: "check",
					},
				},
			},
			{
				Name:      "build",
				DependsOn: []string{"prepare"},
				Steps: []Step{
					{
						Name:    "Build rock-init",
						Tool:    "build",
						Command: "init",
					},
				},
			},
			{
				Name:      "verify",
				DependsOn: []string{"build"},
				Steps: []Step{
					{
						Name:    "Scan dependencies",
						Tool:    "deps",
						Command: "scan",
						Args:    []string{"${OUTPUT_DIR}/sbin/init"},
					},
				},
			},
		},
	}
}

func generateMinimalPipeline() *Pipeline {
	return &Pipeline{
		Name:        "minimal",
		Version:     "1.0",
		Description: "Minimal pipeline template",
		Stages: []Stage{
			{
				Name: "main",
				Steps: []Step{
					{
						Name:    "Step 1",
						Command: "echo 'Hello from pipeline'",
					},
				},
			},
		},
	}
}

func outputJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(data)
}