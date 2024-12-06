package supervisor

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type Supervisor struct {
	processes map[string]*Process
	programs  []*tea.Program
	mu        sync.RWMutex
}

func NewSupervisor() *Supervisor {
	return &Supervisor{
		processes: make(map[string]*Process),
		programs:  make([]*tea.Program, 0),
	}
}

func (s *Supervisor) broadcast(msg tea.Msg) {
	s.mu.RLock()
	programs := make([]*tea.Program, len(s.programs))
	copy(programs, s.programs)
	s.mu.RUnlock()

	for _, p := range programs {
		if p != nil {
			p.Send(msg)
		}
	}
}
