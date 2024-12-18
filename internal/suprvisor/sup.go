package suprvisor

import (
	"bufio"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"walross/nixtea/internal/nixapi"

	"github.com/charmbracelet/log"
)

type UnderSupervision struct {
	items map[string]*Runnable
	mu    sync.RWMutex
}

type Runnable struct {
	Name       string
	BinaryPath string
	Status     string
	PID        int
	buildError error
	process    *ProcessState
}

type BuildError struct {
	Failed  map[string]error
	Success []string
}

type ProcessState struct {
	Cmd       *exec.Cmd
	Done      chan error
	StartTime time.Time
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("%d packages failed to build", len(e.Failed))
}

func NewSupervisor() *UnderSupervision {
	return &UnderSupervision{
		items: make(map[string]*Runnable),
	}
}

// Hydrate updates the supervisor's state with packages from the provided repo URL.
// It creates a Runnable entry for each package in the flake and builds them.
func (s *UnderSupervision) Hydrate(repoURL string) error {

	s.mu.Lock()
	defer s.mu.Unlock()

	client := nixapi.NewClient()
	defer client.Close()

	packages, err := client.GetSystemPackages(repoURL)
	if err != nil {
		return fmt.Errorf("failed to get packages: %w", err)
	}

	// Reset the items map to start fresh
	s.items = make(map[string]*Runnable)

	buildError := &BuildError{
		Failed:  make(map[string]error),
		Success: make([]string, 0),
	}

	for key, pkg := range packages {
		log.Info("Building package", "name", pkg.Name, "key", key)

		runnable := &Runnable{
			Name:   pkg.Name,
			Status: "stopped",
			PID:    0,
		}
		s.items[key] = runnable

		buildResult, err := client.BuildPkg(repoURL, key)
		if err != nil {
			log.Error("Failed to build package",
				"name", pkg.Name,
				"key", key,
				"error", err)

			buildError.Failed[key] = err
			runnable.buildError = err
			runnable.Status = "build_failed"
			continue
		}

		runnable.BinaryPath = buildResult.BinaryPath
		buildError.Success = append(buildError.Success, key)

		log.Info("Successfully built package",
			"name", pkg.Name,
			"key", key,
			"binary", buildResult.BinaryPath)
	}

	if len(buildError.Failed) > 0 {
		return buildError
	}

	s.debugAfterOperation("hydrate")

	return nil
}

// Run starts a package by its key
func (s *UnderSupervision) Run(key string) error {
	s.mu.Lock()
	runnable, exists := s.items[key]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("package %s not found", key)
	}

	// Check if already running
	if runnable.Status == "running" {
		return fmt.Errorf("package %s is already running", key)
	}

	// Check if build failed
	if runnable.Status == "build_failed" {
		return fmt.Errorf("package %s failed to build, cannot run", key)
	}

	// Check if we have a binary path
	if runnable.BinaryPath == "" {
		return fmt.Errorf("no binary path for package %s", key)
	}

	// Create command
	cmd := exec.Command(runnable.BinaryPath)

	// Create a new process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set up stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Set up stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Create process state
	processState := &ProcessState{
		Cmd:       cmd,
		Done:      make(chan error, 1),
		StartTime: time.Now(),
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Update state
	s.mu.Lock()
	runnable.process = processState
	runnable.Status = "running"
	runnable.PID = cmd.Process.Pid
	s.mu.Unlock()

	// Handle stdout in a goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Info("stdout",
				"package", key,
				"pid", cmd.Process.Pid,
				"message", scanner.Text())
		}
	}()

	// Handle stderr in a goroutine
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Info("stderr",
				"package", key,
				"pid", cmd.Process.Pid,
				"message", scanner.Text())
		}
	}()

	// Monitor process in a goroutine
	go func() {
		err := cmd.Wait()

		s.mu.Lock()
		runnable.Status = "stopped"
		runnable.PID = 0
		s.mu.Unlock()

		log.Info("Process exited",
			"package", key,
			"error", err)

		processState.Done <- err
	}()

	log.Info("Process started",
		"package", key,
		"pid", cmd.Process.Pid,
		"binary", runnable.BinaryPath)

	// Debug state after starting
	s.debugAfterOperation("run")

	return nil
}

// Stop stops a running package
func (s *UnderSupervision) Stop(key string) error {
	s.mu.Lock()
	runnable, exists := s.items[key]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("package %s not found", key)
	}

	if runnable.Status != "running" || runnable.process == nil {
		return fmt.Errorf("package %s is not running", key)
	}

	// Get the process group ID
	pgid, err := syscall.Getpgid(runnable.PID)
	if err != nil {
		return fmt.Errorf("failed to get process group: %w", err)
	}

	// Send SIGTERM to the process group
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to terminate process: %w", err)
	}

	// Wait for process to exit with timeout
	select {
	case err := <-runnable.process.Done:
		if err != nil && err.Error() != "signal: terminated" {
			return fmt.Errorf("process terminated with error: %w", err)
		}
	case <-time.After(5 * time.Second):
		// Force kill if timeout
		log.Warn("Process didn't terminate gracefully, forcing kill",
			"package", key,
			"pid", runnable.PID)
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to force kill process: %w", err)
		}
	}

	// Update state
	s.mu.Lock()
	runnable.Status = "stopped"
	runnable.PID = 0
	runnable.process = nil
	s.mu.Unlock()

	log.Info("Process stopped", "package", key)
	s.debugAfterOperation("stop")

	return nil
}

func (s *UnderSupervision) Status(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[name]
	if !exists {
		return "", fmt.Errorf("no service found: %s", name)
	}

	return item.Status, nil
}

// ItemState represents the public state of a runnable item
type ItemState struct {
	Name       string
	Status     string
	Pid        int
	BinaryPath string
	StorePath  string
}

// GetItems returns a copy of the current items map with public state
func (s *UnderSupervision) GetSupervised() map[string]ItemState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make(map[string]ItemState)
	for key, item := range s.items {
		items[key] = ItemState{
			Name:       item.Name,
			Status:     item.Status,
			Pid:        item.PID,
			BinaryPath: item.BinaryPath,
		}
	}
	return items
}

// HasItems returns true if the supervisor has any items
func (s *UnderSupervision) HasItems() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items) > 0
}
