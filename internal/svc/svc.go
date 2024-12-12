// file: internal/svc/svc.go
package svc

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/kardianos/service"
)

type Program struct {
	binPath string
	cmd     *exec.Cmd
	quit    chan struct{}
}

func NewProgram(binPath string) *Program {
	return &Program{
		binPath: binPath,
		quit:    make(chan struct{}),
	}
}

// Start implements service.Interface
func (p *Program) Start(s service.Service) error {
	go p.run()
	return nil
}

// Stop implements service.Interface
func (p *Program) Stop(s service.Service) error {
	close(p.quit)
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Signal(os.Interrupt) // Most Nix binaries handle this well
	}
	return nil
}

func (p *Program) run() {
	for {
		select {
		case <-p.quit:
			return
		default:
			fmt.Printf("svc.run %s", p.binPath)
			cmd := exec.Command(p.binPath)
			cmd.Stdout = os.Stdout // Let systemd/service manager handle logging
			cmd.Stderr = os.Stderr
			p.cmd = cmd

			fmt.Printf("std out %s", p.cmd.Stdout)
			fmt.Printf("std err %s", p.cmd.Stderr)

			if err := cmd.Run(); err != nil {
				fmt.Printf("Process exited with error: %v\n", err)
			}
		}
	}
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

func (m *Manager) Install(name, binPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Basic sanity check
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found at %s: %w", binPath, err)
	}

	program := NewProgram(binPath)
	config := &service.Config{
		Name:        fmt.Sprintf("nixtea-%s", name),
		DisplayName: fmt.Sprintf("Nixtea - %s", name),
		Description: fmt.Sprintf("Nix package service for %s", name),
	}

	svc, err := service.New(program, config)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	m.services[name] = svc
	return nil
}

func (m *Manager) Start(name string) error {
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not installed", name)
	}
	return svc.Run()
}

func (m *Manager) Stop(name string) error {
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not installed", name)
	}
	return svc.Stop()
}

func (m *Manager) Status(name string) (service.Status, error) {
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		return service.Status(0), fmt.Errorf("service %s not installed", name)
	}
	return svc.Status()
}
