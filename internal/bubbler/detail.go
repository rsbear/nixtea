package bubbler

import (
	"fmt"
	"strings"
	"time"

	"tinyship/peanuts/internal/nixapi"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#323232"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}

	titleStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(subtle).
			Padding(0, 1)

	logsContainerStyle = lipgloss.NewStyle().Padding(0, 1)

	columnStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(subtle).
			Padding(1, 2)

	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("25")).
			Padding(0, 1)
)

type LogLine struct {
	Text      string
	Timestamp time.Time
}

type DetailState struct {
	pkg          nixapi.PackageDisplay
	logsViewport viewport.Model
	outputLines  []LogLine
	Pid          int
}

func (m model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			log.Info("Running process", "pkg", m.detailState.pkg.Name)
			if err := m.sv.StartService(m.detailState.pkg.Name, m.detailState.pkg.Key, m.inputState.urlInput); err != nil {
				m.detailState.outputLines = append(m.detailState.outputLines, LogLine{
					Text:      fmt.Sprintf("Error starting service: %v", err),
					Timestamp: time.Now(),
				})
			} else {
				m.detailState.outputLines = append(m.detailState.outputLines, LogLine{
					Text:      "Service started successfully",
					Timestamp: time.Now(),
				})
			}
			m.updateLogsViewport()
			return m, nil

		case "s":
			if err := m.sv.StopService(m.detailState.pkg.Key); err != nil {
				m.detailState.outputLines = append(m.detailState.outputLines, LogLine{
					Text:      fmt.Sprintf("Error stopping service: %v", err),
					Timestamp: time.Now(),
				})
			} else {
				m.detailState.outputLines = append(m.detailState.outputLines, LogLine{
					Text:      "Service stopped successfully",
					Timestamp: time.Now(),
				})
			}
			m.updateLogsViewport()
			return m, nil

		case "esc":
			m.currentPane = PaneList
			return m, nil

		default:
			return m.updateDetailViewport(msg)
		}
	}
	return m, nil
}

// Helper function to update the viewport content
func (m *model) updateLogsViewport() {
	textLines := make([]string, len(m.detailState.outputLines))
	for i, line := range m.detailState.outputLines {
		textLines[i] = line.Text
	}
	m.detailState.logsViewport.SetContent(strings.Join(textLines, "\n"))
	m.detailState.logsViewport.GotoBottom()
}

func (m model) updateDetailViewport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "up", "k":
		m.detailState.logsViewport.LineUp(1)
	case "down", "j":
		m.detailState.logsViewport.LineDown(1)
	case "pgup":
		m.detailState.logsViewport.HalfViewUp()
	case "pgdown":
		m.detailState.logsViewport.HalfViewDown()
	}
	return m, cmd
}

func (m model) viewDetail() string {
	// Calculate dimensions
	totalWidth := m.width
	logsWidth := (totalWidth * 2) / 3
	metricsWidth := totalWidth - logsWidth - 3
	contentHeight := m.height - 10

	// Initialize viewport if needed
	if m.detailState.logsViewport.Width == 0 {
		m.detailState.logsViewport = viewport.New(logsWidth-4, contentHeight-3)
		m.updateLogsViewport()
	}

	// Create logs section
	logsTitle := titleStyle.Width(logsWidth - 4).Render("Logs")
	logsContent := m.detailState.logsViewport.View()
	logsColumn := logsContainerStyle.Width(logsWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			logsTitle,
			logsContent,
		),
	)

	// Get process metadata and create metrics content
	var metricsContent string
	metadata, err := m.sv.GetMetadata(m.detailState.pkg.Key)

	if err != nil {
		// Process hasn't been started yet
		metricsContent = strings.Join([]string{
			fmt.Sprintf("Package: %s", m.detailState.pkg.Name),
			fmt.Sprintf("Key: %s", m.detailState.pkg.Key),
			fmt.Sprintf("Status: %s", lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Not Started")),
		}, "\n")
	} else {
		// Define status style based on running state
		var statusStyle lipgloss.Style
		var statusText string
		if metadata.IsRunning {
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
			statusText = "Running"
		} else {
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
			statusText = "Stopped"
		}

		// Build metrics content with all available metadata
		metrics := []string{
			fmt.Sprintf("Package: %s", m.detailState.pkg.Name),
			fmt.Sprintf("Key: %s", m.detailState.pkg.Key),
			fmt.Sprintf("Status: %s", statusStyle.Render(statusText)),
			"",
		}

		if metadata.IsRunning {
			metrics = append(metrics,
				fmt.Sprintf("PID: %d", metadata.PID),
				fmt.Sprintf("Start Time: %s", metadata.StartTime.Format("15:04:05")),
				fmt.Sprintf("Uptime: %s", metadata.Uptime),
				fmt.Sprintf("Memory Usage: %s", metadata.MemoryUsage),
				fmt.Sprintf("CPU Usage: %s", metadata.CPUUsage),
			)
		}

		metricsContent = strings.Join(metrics, "\n")
	}

	// Create metrics section
	metricsTitle := titleStyle.Width(metricsWidth - 4).Render("Metrics")
	metricsColumn := columnStyle.Width(metricsWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			metricsTitle,
			metricsContent,
		),
	)

	// Combine columns
	columns := lipgloss.JoinHorizontal(lipgloss.Top, logsColumn, metricsColumn)

	// Build final view
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewHeader(),
		"",
		columns,
		"",
		m.viewFooter("↑/k,↓/j: scroll • pgup/pgdown: page scroll • r: run • s: stop • esc: back • q: quit"),
	)
}
