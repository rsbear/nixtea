// file: internal/supervisor/program.go
//
// a program is a tea.Program that can be added to the supervisor
// really its a an ssh session

package supervisor

import tea "github.com/charmbracelet/bubbletea"

func (s *Supervisor) AddProgram(p *tea.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.programs = append(s.programs, p)
}

func (s *Supervisor) RemoveProgram(p *tea.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, prog := range s.programs {
		if prog == p {
			s.programs = append(s.programs[:i], s.programs[i+1:]...)
			break
		}
	}
}
