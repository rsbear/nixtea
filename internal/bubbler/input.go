package bubbler

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

type InputState struct {
	urlInput   string
	focused    bool
	errorMsg   string
	successMsg string
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.inputState.focused {
			if err := m.db.SaveRepoURL(m.inputState.urlInput); err != nil {
				m.inputState.errorMsg = fmt.Sprintf("Failed to save: %v", err)
				return m, nil
			}

			packages, err := m.nixClient.GetFormattedPackages(m.inputState.urlInput)
			if err != nil {
				m.inputState.errorMsg = fmt.Sprintf("Failed to get packages: %v", err)
				return m, nil
			}

			m.listState.packages = packages
			m.currentPane = PaneList
		}
	case "backspace":
		if m.inputState.focused && len(m.inputState.urlInput) > 0 {
			m.inputState.urlInput = m.inputState.urlInput[:len(m.inputState.urlInput)-1]
		}
	default:
		if m.inputState.focused {
			m.inputState.urlInput += msg.String()
		}
	}
	return m, nil
}

func (m model) viewInput() string {
	var s string
	s += m.viewHeader()

	inputPrompt := "Repository URL: "
	if m.inputState.focused {
		inputPrompt = "Repository URL: " + m.inputState.urlInput
		s += termenv.String(inputPrompt).Reverse().String()
	} else {
		s += inputPrompt + m.inputState.urlInput
	}

	if m.inputState.errorMsg != "" {
		s += "\n" + termenv.String(m.inputState.errorMsg).Foreground(termenv.ANSIBrightRed).String()
	}

	s += m.viewFooter("tab: focus input • enter: submit • q: quit")
	return s
}
