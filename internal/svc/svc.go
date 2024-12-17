package svc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/kardianos/service"
)

// ServiceConfig holds the configuration for a service
type ServiceConfig struct {
	Name        string
	DisplayName string
	Description string
	Executable  string
}

// Manager handles service lifecycle operations
type Manager struct {
	mu          sync.RWMutex
	services    map[string]service.Service
	configCache map[string]*service.Config
}

// NewManager creates a new service manager
func NewManager() *Manager {
	return &Manager{
		services:    make(map[string]service.Service),
		configCache: make(map[string]*service.Config),
	}
}

// formatServiceName ensures consistent service naming
func formatServiceName(name string) string {
	// Ensure the name starts with our prefix
	if !strings.HasPrefix(name, "nixtea-") {
		name = "nixtea-" + name
	}
	// Sanitize the name by replacing invalid characters
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return name
}

// Install creates and registers a new service
func (m *Manager) Install(name, execPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	serviceName := formatServiceName(name)
	log.Info("Installing service", "name", serviceName, "exec", execPath)

	// Verify the executable exists
	if _, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("executable not found at %s: %w", execPath, err)
	}

	// Create program config
	svcConfig := &service.Config{
		Name:             serviceName,
		DisplayName:      fmt.Sprintf("Nixtea - %s", name),
		Description:      fmt.Sprintf("Nix package service for %s", name),
		Executable:       execPath,
		WorkingDirectory: filepath.Dir(execPath),
		Option: service.KeyValue{
			// Ensure service restarts on failure
			"Restart":        "always",
			"RestartSec":     "10",
			"StandardOutput": "journal",
			"StandardError":  "journal",
		},
	}

	// Create the service
	svc, err := service.New(nil, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Store the service and its config
	m.services[serviceName] = svc
	m.configCache[serviceName] = svcConfig

	// Uninstall any existing service before installing
	if err := svc.Uninstall(); err != nil && !strings.Contains(err.Error(), "not installed") {
		log.Warn("Failed to uninstall existing service", "error", err)
	}

	// Install the new service
	if err := svc.Install(); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	log.Info("Service installed successfully", "name", serviceName)
	return nil
}

// Start starts a service
func (m *Manager) Start(name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	serviceName := formatServiceName(name)
	svc, exists := m.services[serviceName]
	if !exists {
		// Try to load existing service
		if cfg, ok := m.configCache[serviceName]; ok {
			var err error
			svc, err = service.New(nil, cfg)
			if err != nil {
				return fmt.Errorf("failed to load service: %w", err)
			}
		} else {
			return fmt.Errorf("service %s not installed", name)
		}
	}

	log.Info("Starting service", "name", serviceName)
	return svc.Start()
}

// Stop stops a service
func (m *Manager) Stop(name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	serviceName := formatServiceName(name)
	svc, exists := m.services[serviceName]
	if !exists {
		// Try to load existing service
		if cfg, ok := m.configCache[serviceName]; ok {
			var err error
			svc, err = service.New(nil, cfg)
			if err != nil {
				return fmt.Errorf("failed to load service: %w", err)
			}
		} else {
			return fmt.Errorf("service %s not installed", name)
		}
	}

	log.Info("Stopping service", "name", serviceName)
	return svc.Stop()
}

// Status gets the status of a service
func (m *Manager) Status(name string) (service.Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	serviceName := formatServiceName(name)
	svc, exists := m.services[serviceName]
	if !exists {
		// Try to load existing service
		if cfg, ok := m.configCache[serviceName]; ok {
			var err error
			svc, err = service.New(nil, cfg)
			if err != nil {
				return service.StatusUnknown, fmt.Errorf("failed to load service: %w", err)
			}
		} else {
			return service.StatusUnknown, fmt.Errorf("service %s not installed", name)
		}
	}

	return svc.Status()
}

// Uninstall removes a service
func (m *Manager) Uninstall(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	serviceName := formatServiceName(name)
	svc, exists := m.services[serviceName]
	if !exists {
		// Try to load existing service
		if cfg, ok := m.configCache[serviceName]; ok {
			var err error
			svc, err = service.New(nil, cfg)
			if err != nil {
				return fmt.Errorf("failed to load service: %w", err)
			}
		} else {
			return fmt.Errorf("service %s not installed", name)
		}
	}

	log.Info("Uninstalling service", "name", serviceName)
	if err := svc.Stop(); err != nil {
		log.Warn("Failed to stop service before uninstall", "error", err)
	}

	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("failed to uninstall service: %w", err)
	}

	delete(m.services, serviceName)
	delete(m.configCache, serviceName)
	return nil
}
