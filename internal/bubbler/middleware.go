package bubbler

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"tinyship/peanuts/internal/db"
	"tinyship/peanuts/internal/nixapi"
	"tinyship/peanuts/internal/supervisor"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/muesli/termenv"
)

type timeMsg time.Time

type Pane int

const (
	PaneInput Pane = iota
	PaneList
	PaneDetail
)

type model struct {
	currentPane Pane
	inputState  InputState
	listState   ListState
	detailState DetailState
	width       int
	height      int
	term        string
	time        time.Time
	db          *db.DB
	nixClient   *nixapi.Client
	program     *tea.Program
	sess        ssh.Session
	sv          *supervisor.Supervisor
}

func BubblerMiddleware(sv *supervisor.Supervisor) wish.Middleware {
	db, err := db.New()
	if err != nil {
		log.Error("Failed to initialize database", "error", err)
		return nil
	}

	nixClient := nixapi.NewClient(getCurrentSystem())

	savedURL, err := db.GetRepoURL()
	if err != nil {
		log.Error("Failed to get saved URL", "error", err)
	}

	newProg := func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		p := tea.NewProgram(m, opts...)
		go func() {
			for {
				<-time.After(1 * time.Second)
				p.Send(timeMsg(time.Now()))
			}
		}()
		return p
	}

	teaHandler := func(s ssh.Session) *tea.Program {
		pty, _, active := s.Pty()
		if !active {
			wish.Fatalln(s, "no active terminal")
			return nil
		}

		m := model{
			currentPane: PaneInput,
			inputState: InputState{
				urlInput: savedURL,
				focused:  true,
			},
			width:     pty.Window.Width,
			height:    pty.Window.Height,
			term:      pty.Term,
			db:        db,
			nixClient: nixClient,
			program:   nil,
			sv:        sv,
		}

		if savedURL != "" {
			packages, err := nixClient.GetFormattedPackages(savedURL)
			if err == nil {
				m.listState.packages = packages
				m.currentPane = PaneList

			}
		}

		p := newProg(m, append(bubbletea.MakeOptions(s), tea.WithAltScreen())...)
		sv.AddProgram(p) // Register this program

		// Clean up when the session ends
		go func() {
			<-s.Context().Done() // Wait for session context to be done
			sv.RemoveProgram(p)
		}()

		return p

	}

	return bubbletea.MiddlewareWithProgramHandler(teaHandler, termenv.ANSI256)
}

func (m model) Init() tea.Cmd {

	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {
	case timeMsg:
		m.time = time.Time(msg)

	case supervisor.NewLogLineMsg:
		// Handle log messages at the top level
		ol := m.detailState.outputLines

		if m.currentPane == PaneDetail {
			m.detailState.outputLines = append(ol, LogLine{
				Text:      msg.Text,
				Timestamp: msg.Timestamp,
			})

			// Sort the lines by timestamp
			sort.Slice(ol, func(i, j int) bool {
				return ol[i].Timestamp.Before(ol[j].Timestamp)
			})

			if m.detailState.logsViewport.Height != 0 {
				// Map the log lines to just their text for display
				textLines := make([]string, len(ol))
				for i, line := range ol {
					textLines[i] = line.Text
				}
				m.detailState.logsViewport.SetContent(strings.Join(textLines, "\n"))
				m.detailState.logsViewport.GotoBottom()
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":

			return m, tea.Quit
		}

		switch m.currentPane {
		case PaneInput:
			return m.updateInput(msg)
		case PaneList:
			return m.updateList(msg)
		case PaneDetail:
			return m.updateDetail(msg)
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.currentPane {
	case PaneInput:
		return m.viewInput()
	case PaneList:
		return m.viewList()
	case PaneDetail:
		return m.viewDetail()
	}
	return ""
}

func getCurrentSystem() string {
	var nixArch, nixOS string

	switch runtime.GOARCH {
	case "amd64":
		nixArch = "x86_64"
	case "arm64":
		nixArch = "aarch64"
	default:
		nixArch = runtime.GOARCH
	}

	switch runtime.GOOS {
	case "darwin":
		nixOS = "darwin"
	case "linux":
		nixOS = "linux"
	default:
		nixOS = runtime.GOOS
	}

	return fmt.Sprintf("%s-%s", nixArch, nixOS)
}

func stringOr(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func (m model) viewHeader() string {
	return fmt.Sprintf("%s %s\n",
		termenv.String(" nixtea ").Background(termenv.ANSIBrightMagenta).Foreground(termenv.ANSIWhite),
		termenv.String(stringOr(m.inputState.urlInput, "repo not set")).Foreground(termenv.ANSIBrightBlack))
}

func (m model) viewFooter(help string) string {
	return fmt.Sprintf("\n%s",
		termenv.String(help).Foreground(termenv.ANSIBrightBlack))
}
