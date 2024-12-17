package cli

import (
	"fmt"
	"io"
	"strings"
	"walross/nixtea/internal/db"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
)

const listHeight = 14

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(mint)
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type repoItem string

func (i repoItem) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(repoItem)
	if !ok {
		return
	}

	str := fmt.Sprintf("• %s", i)
	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}
	fmt.Fprint(w, fn(str))
}

type repoModel struct {
	list     list.Model
	choice   string
	quitting bool
	db       *db.DB
	term     string
	width    int
	height   int
}

func (m repoModel) Init() tea.Cmd {
	return nil
}

func (m repoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			i, ok := m.list.SelectedItem().(repoItem)
			if ok {
				m.choice = string(i)

				_, err := m.db.SaveRepo(string(i))
				if err != nil {
					// Handle error gracefully in the view
					m.choice = "error:" + err.Error()
				}
			}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m repoModel) View() string {
	if m.choice != "" {
		if strings.HasPrefix(m.choice, "error:") {
			return quitTextStyle.Render(fmt.Sprintf("Error: %s", m.choice[6:]))
		}
		return quitTextStyle.Render(fmt.Sprintf("Selected repository: %s", m.choice))
	}
	if m.quitting {
		return quitTextStyle.Render("No repository selected.")
	}
	return "\n" + m.list.View()
}

func handleContextManager(s ssh.Session, db *db.DB) error {

	// Get all repositories
	repos, err := db.GetRepos()
	if err != nil {
		return fmt.Errorf("failed to get repositories: %w", err)
	}

	// If no repositories exist, show help message and exit
	if len(repos) == 0 {
		fmt.Fprintln(s, "No repositories found.")
		fmt.Fprintln(s, "To add a repository, use: nixtea ctx add <url>")
		return nil
	}

	pty, _, active := s.Pty()
	if !active {
		return fmt.Errorf("no active terminal, please use: ssh -t")
	}

	// Get the current URL to highlight it
	currentURL, err := db.GetRepoURL()
	if err != nil {
		return fmt.Errorf("failed to get current repository: %w", err)
	}

	// Convert repos to list items
	items := make([]list.Item, len(repos))
	defaultIndex := 0
	for i, repo := range repos {
		items[i] = repoItem(repo.URL)
		if repo.URL == currentURL {
			defaultIndex = i
		}
	}

	const defaultWidth = 40
	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "Available Repositories"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle
	l.Select(defaultIndex)

	m := repoModel{
		list:   l,
		db:     db,
		term:   pty.Term,
		width:  pty.Window.Width,
		height: pty.Window.Height,
	}

	p := tea.NewProgram(
		m,
		tea.WithInput(s),
		tea.WithOutput(s),
	)

	_, err = p.Run()
	return err
}
