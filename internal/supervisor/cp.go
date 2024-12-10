// file: internal/supervisor/cp.go
//
// cp = child process
// a cp is a process that is spawned by the supervisor
// intended to be a nix-shell process and log running, meaning this app needs to stay on

package supervisor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
)

type NewLogLineMsg struct {
	ProcessKey string
	Text       string
	Timestamp  time.Time
}

type ProcessMetadata struct {
	IsRunning   bool      `json:"is_running"`
	StartTime   time.Time `json:"start_time"`
	Uptime      string    `json:"uptime"`
	MemoryUsage string    `json:"memory_usage"`
	CPUUsage    string    `json:"cpu_usage"`
	PID         int       `json:"pid"`
}

type Process struct {
	Cmd        *exec.Cmd
	Done       chan error
	output     []string
	mu         sync.RWMutex
	pgid       int
	isRunning  bool
	startTime  time.Time
	lastUpdate time.Time
}

// /////////////////////////////////////////////////////
// cp methods
// /////////////////////////////////////////////////////
func (s *Supervisor) StartService(name, key, repoURL string) error {
	// Check if process already exists and is running
	s.mu.Lock()
	if proc, exists := s.processes[key]; exists && proc.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("service %s is already running", name)
	}
	s.mu.Unlock()

	s.logProcessMapState("Before starting service")

	// Create command with proper nix run syntax
	cmd := exec.Command("nix", "run", "--no-write-lock-file", fmt.Sprintf("%s#%s", repoURL, key))

	// Create a new process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set up stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe error: %w", err)
	}

	// Set up stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe error: %w", err)
	}

	proc := &Process{
		Cmd:       cmd,
		Done:      make(chan error, 1),
		isRunning: true,
		startTime: time.Now(),
	}

	s.mu.Lock()
	s.processes[key] = proc
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		delete(s.processes, key)
		s.mu.Unlock()
		return fmt.Errorf("failed to start: %w", err)
	}

	// Store the process group ID after the process has started
	proc.pgid, err = syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("failed to get process group: %w", err)
	}

	// Handle stdout in a goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			fmt.Println("stdout", text)
			s.broadcast(NewLogLineMsg{
				ProcessKey: key,
				Text:       text,
				Timestamp:  time.Now(),
			})

			proc.mu.Lock()
			proc.output = append(proc.output, text)
			proc.mu.Unlock()
		}
	}()

	// Handle stderr in a goroutine
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			text := scanner.Text()
			fmt.Println("stderr", text)
			s.broadcast(NewLogLineMsg{
				ProcessKey: key,
				Text:       fmt.Sprintf("[stderr] %s", text),
				Timestamp:  time.Now(),
			})

			proc.mu.Lock()
			proc.output = append(proc.output, text)
			proc.mu.Unlock()
		}
	}()

	// Monitor process completion in a goroutine
	go func() {
		err := cmd.Wait()
		proc.mu.Lock()
		proc.isRunning = false
		proc.mu.Unlock()

		s.broadcast(NewLogLineMsg{
			ProcessKey: key,
			Text:       fmt.Sprintf("Process exited: %v", err),
			Timestamp:  time.Now(),
		})

		// Send completion status to Done channel
		proc.Done <- err

		// Keep the process in the map but marked as not running
		// This allows viewing logs after completion
		log.Info("Process completed", "key", key, "error", err)
		s.logProcessMapState("After process completion")
	}()

	log.Info("Added process to map", "key", key, "pid", cmd.Process.Pid)
	s.logProcessMapState("After starting service")

	return nil
}

func (s *Supervisor) StopService(key string) error {
	s.mu.Lock()
	proc, exists := s.processes[key]
	s.mu.Unlock()

	if !exists || proc == nil {
		return fmt.Errorf("no process found for key %s", key)
	}

	if !proc.isRunning {
		return fmt.Errorf("process is not running")
	}

	// Send SIGTERM to the process group
	err := syscall.Kill(-proc.pgid, syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to terminate process: %w", err)
	}

	// Wait for process to exit with timeout
	select {
	case err := <-proc.Done:
		if err != nil && err.Error() != "signal: terminated" {
			return fmt.Errorf("process terminated with error: %w", err)
		}
	case <-time.After(5 * time.Second):
		// Force kill if timeout
		log.Warn("Process didn't terminate gracefully, forcing kill")
		if err := syscall.Kill(-proc.pgid, syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to force kill process: %w", err)
		}
	}

	proc.isRunning = false

	log.Info("Process stopped", "key", key)
	s.logProcessMapState("After stopping service")

	return nil
}

// Add helper method to log the state of the processes map
func (s *Supervisor) logProcessMapState(context string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Info(context, "process_count", len(s.processes))
	for key, proc := range s.processes {
		isRunning := "not running"
		pid := 0
		if proc != nil {
			isRunning = fmt.Sprintf("%v", proc.isRunning)
			if proc.Cmd != nil && proc.Cmd.Process != nil {
				pid = proc.Cmd.Process.Pid
			}
		}
		log.Info("Process state",
			"key", key,
			"running", isRunning,
			"pid", pid,
			"nil_proc", proc == nil,
			"nil_cmd", proc != nil && proc.Cmd == nil,
		)
	}
}

// GetMetadata retrieves the current metadata for a process
func (s *Supervisor) GetMetadata(key string) (*ProcessMetadata, error) {
	s.mu.RLock()
	proc, exists := s.processes[key]
	s.mu.RUnlock()

	if !exists || proc == nil {
		return nil, fmt.Errorf("no process found for key %s", key)
	}

	metadata := proc.getMetadata()
	return &metadata, nil
}

func (s *Supervisor) ServicePkgLogs(key string) (chan string, error) {
	logChan := make(chan string)

	s.mu.RLock()
	proc, exists := s.processes[key]
	s.mu.RUnlock()

	if !exists || proc == nil {
		return nil, fmt.Errorf("no process found for key %s", key)
	}

	procFile := fmt.Sprintf("/proc/%d/fd/1", proc.Cmd.Process.Pid)
	file, err := os.Open(procFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open process stdout: %w", err)
	}

	go func() {
		defer file.Close()
		defer close(logChan)

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			logChan <- scanner.Text()
		}
	}()

	return logChan, nil
}

func (s *Supervisor) GetProcess(key string) (*Process, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	proc, exists := s.processes[key]
	return proc, exists
}

// /////////////////////////////////////////////////////
// metadata helpers
// /////////////////////////////////////////////////////
func (p *Process) getMetadata() ProcessMetadata {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := ProcessMetadata{
		IsRunning: p.isRunning,
		StartTime: p.startTime,
	}

	if p.isRunning && p.Cmd != nil && p.Cmd.Process != nil {
		metadata.PID = p.Cmd.Process.Pid
		metadata.Uptime = time.Since(p.startTime).Round(time.Second).String()

		// Get memory and CPU usage based on OS
		switch runtime.GOOS {
		case "linux":
			metadata.MemoryUsage = p.getLinuxMemoryUsage()
			metadata.CPUUsage = p.getLinuxCPUUsage()
		case "darwin":
			metadata.MemoryUsage = p.getDarwinMemoryUsage()
			metadata.CPUUsage = p.getDarwinCPUUsage()
		default:
			metadata.MemoryUsage = "Unsupported OS"
			metadata.CPUUsage = "Unsupported OS"
		}
	}

	return metadata
}

func (p *Process) getLinuxMemoryUsage() string {
	cmd := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(p.Cmd.Process.Pid))
	output, err := cmd.Output()
	if err != nil {
		return "N/A"
	}

	memKB, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return "N/A"
	}

	return fmt.Sprintf("%.1f MB", memKB/1024)
}

func (p *Process) getLinuxCPUUsage() string {
	cmd := exec.Command("ps", "-o", "%cpu=", "-p", strconv.Itoa(p.Cmd.Process.Pid))
	output, err := cmd.Output()
	if err != nil {
		return "N/A"
	}

	return strings.TrimSpace(string(output)) + "%"
}

func (p *Process) getDarwinMemoryUsage() string {
	cmd := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(p.Cmd.Process.Pid))
	output, err := cmd.Output()
	if err != nil {
		return "N/A"
	}

	memKB, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return "N/A"
	}

	return fmt.Sprintf("%.1f MB", memKB/1024)
}

func (p *Process) getDarwinCPUUsage() string {
	cmd := exec.Command("ps", "-o", "%cpu=", "-p", strconv.Itoa(p.Cmd.Process.Pid))
	output, err := cmd.Output()
	if err != nil {
		return "N/A"
	}

	return strings.TrimSpace(string(output)) + "%"
}
