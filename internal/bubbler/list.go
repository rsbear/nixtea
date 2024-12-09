package bubbler

import (
	"fmt"
	"walross/nixtea/internal/nixapi"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

type ListState struct {
	packages      []nixapi.PackageDisplay
	selectedIndex int
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.listState.selectedIndex > 0 {
			m.listState.selectedIndex--
		}
	case "down":
		if m.listState.selectedIndex < len(m.listState.packages)-1 {
			m.listState.selectedIndex++
		}
	case "u", "U":
		// Start update in a goroutine to avoid blocking UI
		go func() {
			if err := m.nixClient.UpdateFlake(m.inputState.urlInput); err != nil {
				m.program.Send(UpdateListFailedMsg{err: err})
				return
			}

			// Reload packages after successful update
			packages, err := m.nixClient.GetFormattedPackages(m.inputState.urlInput)
			if err != nil {
				m.program.Send(UpdateListFailedMsg{err: err})
				return
			}

			m.program.Send(UpdateListSuccessMsg{packages: packages})
		}()
		return m, nil

	case "enter":
		if len(m.listState.packages) > 0 {
			pkg := m.listState.packages[m.listState.selectedIndex]
			m.detailState.pkg = pkg

			// Initialize viewport
			vp := viewport.New(m.width, m.height-6)
			vp.SetContent("")

			m.currentPane = PaneDetail
		}
	case "esc":
		m.currentPane = PaneInput
	}
	return m, nil
}

func (m model) viewList() string {
	var s string
	s += m.viewHeader()

	// Calculate available space for list
	contentHeight := m.height - 4 // Account for header (2) and footer (2)

	for i, pkg := range m.listState.packages {
		if i >= contentHeight {
			break
		}

		// Get actual state if we have a PID
		var stateStr string

		line := fmt.Sprintf("• %s%s %s",
			pkg.Name,
			stateStr,
			termenv.String("#"+pkg.Key).Foreground(termenv.ANSIBrightBlack))

		if i == m.listState.selectedIndex {
			s += termenv.String(line).Foreground(termenv.ANSIGreen).String() + "\n"
		} else {
			s += line + "\n"
		}
	}

	s += m.viewFooter("↑/↓: navigate • enter: select • U: update • esc: back • q: quit")
	return s
}
