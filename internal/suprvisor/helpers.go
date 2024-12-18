package suprvisor

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// DebugState prints the current state of the supervisor to the logs
func (s *UnderSupervision) DebugState() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder

	// Create header
	b.WriteString("\n=== Supervisor State Debug ===\n")
	b.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Total Items: %d\n", len(s.items)))
	b.WriteString("===========================\n\n")

	// If no items, show empty state
	if len(s.items) == 0 {
		b.WriteString("No items under supervision\n")
	}

	// Print details for each item
	for key, item := range s.items {
		b.WriteString(fmt.Sprintf("Package: %s\n", key))
		b.WriteString(fmt.Sprintf("  Name: %s\n", item.Name))
		b.WriteString(fmt.Sprintf("  Status: %s\n", item.Status))
		b.WriteString(fmt.Sprintf("  PID: %d\n", item.PID))
		b.WriteString(fmt.Sprintf("  Binary Path: %s\n", stringOrNA(item.BinaryPath)))

		if item.buildError != nil {
			b.WriteString(fmt.Sprintf("  Build Error: %v\n", item.buildError))
		}
		b.WriteString("\n")
	}

	b.WriteString("===========================\n")

	// Log the entire state at once
	log.Info("Supervisor Debug State", "state", b.String())
}

// Helper function for printing state
func stringOrNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

// Debug logs a single item's state
func (s *UnderSupervision) DebugItem(key string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[key]
	if !exists {
		log.Info("Debug Item Not Found",
			"key", key,
			"error", "item does not exist")
		return
	}

	log.Info("Package Debug Info",
		"key", key,
		"name", item.Name,
		"status", item.Status,
		"pid", item.PID,
		"binary_path", stringOrNA(item.BinaryPath),
		"build_error", item.buildError)
}

// Add debug helper to print state after key operations
func (s *UnderSupervision) debugAfterOperation(op string) {
	if log.GetLevel() >= log.DebugLevel {
		log.Debug("Supervisor operation completed", "operation", op)
		s.DebugState()
	}
}
