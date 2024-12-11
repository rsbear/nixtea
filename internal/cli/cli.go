// internal/cli/cli.go

package cli

import (
	"fmt"
	"walross/nixtea/internal/config"
	"walross/nixtea/internal/supervisor"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	ns  = lipgloss.NewStyle()
	row = lipgloss.NewStyle().MarginLeft(2)

	// colours
	purp     = lipgloss.Color("#9D8CFF")
	white100 = lipgloss.Color("#FFFFFF")
	white80  = lipgloss.Color("#666666")
	white40  = lipgloss.Color("#333333")
	white10  = lipgloss.Color("#323232")
)

func NewRootCmd(sv *supervisor.Supervisor, cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "nixtea",
		Short: "Nixtea is a Nix package runner and manager",
	}

	// ctx - list/add/set repos
	ctxCmd := &cobra.Command{
		Use:   "ctx",
		Short: "Manage repository contexts",
		RunE: func(cmd *cobra.Command, args []string) error {
			// For now just return some basic info
			cmd.Println(row.
				Width(40).
				PaddingTop(1).
				Foreground(purp).Bold(true).
				BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(white10).
				Render("Nixtea"))

			label := ns.Foreground(white80).SetString("current ctx -")
			currentCtx := ns.Foreground(white100).SetString("github:todo/implement")
			ln := label.Render() + " " + currentCtx.Render()

			// render ctx hint
			cmd.Println(row.PaddingBottom(2).Render(ln))

			cmd.Println("Repository management - TODO")
			cmd.Println("- List saved repos")
			cmd.Println("- Add new repo")
			cmd.Println("- Set active repo")
			return nil
		},
	}

	// ps - list packages
	psCmd := &cobra.Command{
		Use:   "ps",
		Short: "List available packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Package listing - TODO")
			cmd.Println("Will show packages from active repo")
			return nil
		},
	}

	// <pkg> run - start package
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]
			cmd.Printf("Will start package: %s\n", pkgKey)
			return nil
		},
	}

	// <pkg> stop - stop package
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a running package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]
			cmd.Printf("Will stop package: %s\n", pkgKey)
			return nil
		},
	}

	// <pkg> status - show metrics
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show package status and metrics",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]
			cmd.Printf("Will show status for package: %s\n", pkgKey)
			cmd.Println("- Process info")
			cmd.Println("- Metrics")
			cmd.Println("- Recent logs")
			return nil
		},
	}

	// <pkg> logs - streaming logs
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream package logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]
			cmd.Printf("Will stream logs for package: %s\n", pkgKey)
			cmd.Println("(Press ESC to exit)")
			return nil
		},
	}

	// help command
	helpCmd := &cobra.Command{
		Use:   "help",
		Short: "Show nixtea help",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("nixtea - A Nix package runner and manager")
			cmd.Println("\nUsage:")
			cmd.Println("  ssh nixtea <command>")
			cmd.Println("\nCommands:")
			cmd.Println("  ctx               List, add, or set active repositories")
			cmd.Println("  ps                List packages from active repository")
			cmd.Println("  <pkg> run         Start a package")
			cmd.Println("  <pkg> stop        Stop a running package")
			cmd.Println("  <pkg> status      Show package status and metrics")
			cmd.Println("  <pkg> logs        Stream package logs (ESC to quit)")
			cmd.Println("  help              Show this help message")
			return nil
		},
	}

	// Add all commands to root
	rootCmd.AddCommand(ctxCmd)
	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)

	return rootCmd
}

// CreateMiddleware creates a wish middleware that handles CLI commands
func CreateMiddleware(sv *supervisor.Supervisor, cfg *config.Config) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			// If no command provided, continue to next middleware (TUI)
			if len(sess.Command()) == 0 {
				next(sess)
				return
			}

			// Set up root command
			rootCmd := NewRootCmd(sv, cfg)
			rootCmd.SetArgs(sess.Command())
			rootCmd.SetIn(sess)
			rootCmd.SetOut(sess)
			rootCmd.SetErr(sess.Stderr())
			rootCmd.CompletionOptions.DisableDefaultCmd = true

			// Execute command
			if err := rootCmd.Execute(); err != nil {
				fmt.Fprintf(sess.Stderr(), "Error: %v\n", err)
				_ = sess.Exit(1)
				return
			}

			_ = sess.Exit(0)
		}
	}
}
