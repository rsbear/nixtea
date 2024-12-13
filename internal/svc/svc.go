// file: internal/svc/svc.go

package svc

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/service"
)

type Program struct {
	binPath   string
	cmd       *exec.Cmd
	quit      chan struct{}
	startTime time.Time
}

type ResourceUsage struct {
	Memory string
	CPU    string
}

func NewProgram(binPath string) *Program {
	return &Program{
		binPath: binPath,
		quit:    make(chan struct{}),
	}
}

// Start implements service.Interface
func (p *Program) Start(s service.Service) error {
	// Start in a goroutine so it doesn't block
	p.startTime = time.Now() // Set the start time when service starts
	go p.run()
	return nil
}

// Stop implements service.Interface
func (p *Program) Stop(s service.Service) error {
	close(p.quit)
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Signal(os.Interrupt)
	}
	return nil
}

func (p *Program) run() {
	cmd := exec.Command(p.binPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	p.cmd = cmd

	// Start the command (non-blocking)
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start process: %v\n", err)
		return
	}

	// Wait for either quit signal or process completion
	go func() {
		<-p.quit
		if p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Signal(os.Interrupt)
		}
	}()

	// Wait for process in a separate goroutine
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Printf("Process exited with error: %v\n", err)
		}
	}()
}

type Manager struct {
	mu       sync.RWMutex
	services map[string]service.Service
}

func NewManager() *Manager {
	return &Manager{
		services: make(map[string]service.Service),
	}
}

// getServiceName ensures consistent service name formatting
func (m *Manager) getServiceName(name string) string {
	// If it already has the prefix, don't add it again
	if strings.HasPrefix(name, "nixtea-") {
		return name
	}
	return "nixtea-" + name
}

func (m *Manager) Install(name, binPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	serviceName := m.getServiceName(name)

	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found at %s: %w", binPath, err)
	}

	program := NewProgram(binPath)
	config := &service.Config{
		Name:        serviceName,
		Description: fmt.Sprintf("Nix package service for %s", name),
		Executable:  binPath,
	}

	svc, err := service.New(program, config)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	fmt.Println("service.New success serviceName", serviceName)

	m.services[serviceName] = svc
	svc.Platform()
	return nil
}

func (m *Manager) Start(name string) error {
	m.mu.RLock()
	serviceName := m.getServiceName(name)
	svc, exists := m.services[serviceName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not installed", name)
	}

	// Start the service in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- svc.Run()
	}()

	status, err := svc.Status()
	fmt.Println("returning status", status)
	fmt.Println("err", err)

	// Wait a short time for any immediate errors
	select {
	case err := <-errChan:
		return fmt.Errorf("service failed to start: %w", err)
	case <-time.After(100 * time.Millisecond):
		// If no immediate error, assume service started successfully
		return nil
	}
}

// grandmother claude
// Status gets the status of a service
func (m *Manager) Status(name string) (service.Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If not found in our managed services, try to find it in system services
	// First try to find it in our managed services
	serviceName := m.getServiceName(name)
	if svc, exists := m.services[serviceName]; exists {
		fmt.Println("found in managed services", svc.String())

		status, err := svc.Status()
		if err != nil {

			fmt.Println("error", err)
			return status, err
		}
		fmt.Println("returning status", status)

		return svc.Status()
	}

	return service.StatusUnknown, fmt.Errorf("service not found")
}

// Stop stops a service
func (m *Manager) Stop(name string) error {
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		// Try to find it in system services
		service.ChooseSystem()

		serviceName := m.getServiceName(name)
		svcConfig := &service.Config{Name: serviceName}

		if s, err := service.New(nil, svcConfig); err == nil {
			return s.Stop()
		}
		return fmt.Errorf("service %s not installed", name)
	}

	return svc.Stop()
}
