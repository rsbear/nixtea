// internal/cli/cli.go

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"walross/nixtea/internal/config"
	"walross/nixtea/internal/db"
	"walross/nixtea/internal/supervisor"
	"walross/nixtea/internal/suprvisor"
	"walross/nixtea/internal/svc"

	"github.com/charmbracelet/lipgloss/tree"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/kardianos/service"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	ns  = lipgloss.NewStyle()
	row = lipgloss.NewStyle().MarginLeft(2)

	// colours
	mint     = lipgloss.Color("#61c9a8")
	purp     = lipgloss.Color("#9D8CFF")
	white100 = lipgloss.Color("#FFFFFF")
	white80  = lipgloss.Color("#666666")
	white40  = lipgloss.Color("#333333")
	white10  = lipgloss.Color("#323232")
)

func titleBlock() string {
	return ns.PaddingLeft(2).PaddingTop(1).PaddingBottom(0).Render("Nixtea")
}

// ctxUpdateCmd creates the 'ctx update' command
func ctxUpdateCmd(cfg *config.Config, db *db.DB, svcMgr *svc.Manager, sp *suprvisor.UnderSupervision) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update and rebuild all packages from current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := db.GetRepoURL()
			if err != nil {
				return fmt.Errorf("failed to get repository URL: %w", err)
			}
			if url == "" {
				return fmt.Errorf("no repository set. Use 'nixtea ctx add' to set a repository")
			}
			cmd.Printf("→ Found active repository: %s\n", url)

			err = sp.Hydrate(url)
			if err != nil {
				// Handle build errors
				if buildErr, ok := err.(*suprvisor.BuildError); ok {
					if len(buildErr.Success) > 0 {
						cmd.Printf("\n✓ Successfully built packages:\n")
						for _, key := range buildErr.Success {
							cmd.Printf("  • %s\n", key)
						}
					}
					if len(buildErr.Failed) > 0 {
						cmd.Printf("\n✗ Failed to build packages:\n")
						for key, err := range buildErr.Failed {
							cmd.Printf("  • %s: %v\n", key, err)
						}
					}
					return nil // Don't propagate the error further
				}
				// Handle other errors
				return fmt.Errorf("failed to hydrate: %w", err)
			}

			cmd.Println("\n✓ All packages built successfully!")
			return nil
		},
	}
}

// Helper function to create ctx add command
func ctxAddCmd(cfg *config.Config, db *db.DB) *cobra.Command {
	return &cobra.Command{
		Use:   "add [url]",
		Short: "Add a new repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			repo, err := db.SaveRepo(url)
			if err != nil {
				return fmt.Errorf("failed to save repository: %w", err)
			}
			cmd.Printf("Added repository %s\n", repo.URL)
			return nil
		},
	}
}

func pksRunCmd(cfg *config.Config, db *db.DB, sp *suprvisor.UnderSupervision) *cobra.Command {
	return &cobra.Command{
		Use:   "run [package]",
		Short: "Run a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]

			// Get current repository URL (needed if we have to hydrate)
			url, err := db.GetRepoURL()
			if err != nil {
				return fmt.Errorf("failed to get repository URL: %w", err)
			}
			if url == "" {
				return fmt.Errorf("no repository set. Use 'nixtea ctx add' to set a repository")
			}

			// If supervisor has no items, hydrate it first
			if !sp.HasItems() {
				cmd.Printf("→ Loading package state...\n")
				if err := sp.Hydrate(url); err != nil {
					return fmt.Errorf("failed to hydrate supervisor: %w", err)
				}
			}

			// Run the package
			cmd.Printf("→ Starting package %s...\n", pkgKey)
			if err := sp.Run(pkgKey); err != nil {
				cmd.Printf("✗ Failed to start package: %v\n", err)
				return nil // Return nil to avoid double error message
			}

			cmd.Printf("✓ Package %s is now running\n\n", pkgKey)
			cmd.Printf("To check package status:\n")
			cmd.Printf("  nixtea pks status %s\n\n", pkgKey)
			cmd.Printf("To view package logs:\n")
			cmd.Printf("  nixtea pks logs %s\n", pkgKey)

			return nil
		},
	}
}

func NewRootCmd(sv *supervisor.Supervisor, cfg *config.Config, db *db.DB, svcMgr *svc.Manager, sp *suprvisor.UnderSupervision) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "nixtea",
		Short: "Nixtea is a Nix package runner",
	}

	outputStyle := lipgloss.NewStyle().
		PaddingTop(1).
		PaddingLeft(2)

	// ctx - list/add/set repos
	ctxCmd := &cobra.Command{
		Use:   "ctx",
		Short: "Manage repository contexts",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := db.GetRepoURL()
			if err != nil {
				return fmt.Errorf("failed to get repository: %w", err)
			}

			var output string
			if url == "" {
				output = "No repository set\n\n" +
					"To set a repository:\n" +
					"  nixtea ctx add <url>"
			} else {
				output = fmt.Sprintf("%s\n\n"+
					"Next step is to run an output from the repo that was set\n"+
					"List the available packages with:\n"+
					"  nixtea pks", url)
			}

			cmd.Println(outputStyle.Render(output))
			return nil
		},
	}

	// Add subcommands to ctx command
	ctxCmd.AddCommand(ctxAddCmd(cfg, db), ctxUpdateCmd(cfg, db, svcMgr, sp))

	// <pkg> run - start package
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			// Get repository URL
			url, err := db.GetRepoURL()
			if err != nil {
				return fmt.Errorf("failed to get repository URL: %w", err)
			}
			if url == "" {
				return fmt.Errorf("no repository set. Use 'nixtea ctx' to set a repository")
			}

			pkgKey := args[0]
			fullPkgURL := fmt.Sprintf("%s#%s", url, pkgKey)

			// Try to start the service first
			err = svcMgr.Start(pkgKey)
			if err == nil {
				cmd.Printf("✓ Service %s is now running\n\n", pkgKey)
				cmd.Printf("To check service status:\n")
				cmd.Printf("  nixtea %s status\n", pkgKey)
				cmd.Printf("\nTo view service logs:\n")
				cmd.Printf("  nixtea %s logs\n", pkgKey)
				return nil
			}

			// If service isn't installed, build and install it
			if strings.Contains(err.Error(), "not installed") {
				cmd.Printf("→ Installing service %s...\n", pkgKey)

				// Build the package
				buildCmd := exec.Command("nix", "build", "--no-write-lock-file", fullPkgURL, "--print-out-paths")
				outBytes, err := buildCmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("build failed: %w\nOutput: %s", err, string(outBytes))
				}

				storePath := strings.TrimSpace(string(outBytes))
				if storePath == "" {
					return fmt.Errorf("no store path returned from build")
				}

				// Find the binary
				binDir := filepath.Join(storePath, "bin")
				entries, err := os.ReadDir(binDir)
				if err != nil {
					return fmt.Errorf("failed to read bin directory: %w", err)
				}

				if len(entries) == 0 {
					return fmt.Errorf("no binaries found in %s", binDir)
				}

				// Use first binary if there's only one, otherwise error
				if len(entries) > 1 {
					cmd.Printf("Multiple binaries found in %s:\n", binDir)
					for _, entry := range entries {
						cmd.Printf("  - %s\n", entry.Name())
					}
					return fmt.Errorf("multiple binaries found, please specify which one to run")
				}

				binPath := filepath.Join(binDir, entries[0].Name())

				// Install and start the service
				cmd.Printf("→ Registering service...\n")
				if err := svcMgr.Install(pkgKey, binPath); err != nil {
					return fmt.Errorf("failed to install service: %w", err)
				}

				cmd.Printf("→ Starting service...\n")
				if err := svcMgr.Start(pkgKey); err != nil {
					return fmt.Errorf("service installed but failed to start: %w", err)
				}

				cmd.Printf("\n✓ Service %s is now running\n\n", pkgKey)
				cmd.Printf("To check service status:\n")
				cmd.Printf("  nixtea %s status\n", pkgKey)
				cmd.Printf("\nTo view service logs:\n")
				cmd.Printf("  nixtea %s logs\n", pkgKey)
				return nil
			}

			// If we got here, the error was something other than "not installed"
			return fmt.Errorf("failed to start service: %w", err)

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

			// Try getting service status
			status, err := svcMgr.Status(pkgKey)
			if err != nil {
				cmd.Printf("Service %s is not installed\n", pkgKey)
				cmd.Println("To install and start the service:")
				cmd.Printf("  nixtea %s run\n", pkgKey)
				return nil
			}

			// Display status information
			cmd.Printf("Service: %s\n", pkgKey)
			switch status {
			case service.StatusRunning:
				cmd.Println("Status: Running")
			case service.StatusStopped:
				cmd.Println("Status: Stopped")
			default:
				cmd.Printf("Status: %s\n", status)
			}

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
			cmd.Println("  pks                List packages from active repository")
			cmd.Println("  <pkg> run         Start a package")
			cmd.Println("  <pkg> stop        Stop a running package")
			cmd.Println("  <pkg> status      Show package status and metrics")
			cmd.Println("  <pkg> logs        Stream package logs (ESC to quit)")
			cmd.Println("  help              Show this help message")
			return nil
		},
	}

	pksCmd(cfg, db, sp).AddCommand(pksRunCmd(cfg, db, sp))

	// Add all commands to root
	rootCmd.AddCommand(ctxCmd)
	rootCmd.AddCommand(pksCmd(cfg, db, sp))
	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)

	return rootCmd
}

// func handleContextManager(s ssh.Session, db *db.DB) error {
// 	pty, _, active := s.Pty()
// 	if !active {
// 		return fmt.Errorf("no active terminal, please use: ssh -t")
// 	}
//
// 	m := newContextModel(db) // Use our new constructor
// 	m.term = pty.Term
// 	m.width = pty.Window.Width
// 	m.height = pty.Window.Height
//
// 	p := tea.NewProgram(
// 		m,
// 		tea.WithInput(s),
// 		tea.WithOutput(s),
// 	)
//
// 	_, err := p.Run()
// 	return err
// }

// formatPackagesTreeFromState creates a tree view from supervisor state
func formatPackagesTreeFromState(sp *suprvisor.UnderSupervision) string {
	// Get all items from supervisor
	items := sp.GetSupervised()

	// Sort packages by name for consistent display
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Create package entries with styled names
	treeItems := make([]any, len(keys))
	for i, key := range keys {
		item := items[key]

		// Create status indicator
		var statusStyle lipgloss.Style
		switch item.Status {
		case "running":
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green
		case "stopped":
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")) // gray
		case "build_failed":
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
		default:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")) // gray
		}

		treeItems[i] = fmt.Sprintf("%s %s %s",
			item.Name,
			hashStyle.Render("#"+key),
			statusStyle.Render(item.Status),
		)
	}

	// Build the tree
	t := tree.Root("⚡ Nixtea Packages").
		Child(treeItems...).
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(enumeratorStyle).
		RootStyle(rootStyle).
		ItemStyle(itmStyle)

	return t.String()
}

// pksCmd creates the 'pks' command
func pksCmd(cfg *config.Config, db *db.DB, sp *suprvisor.UnderSupervision) *cobra.Command {
	return &cobra.Command{
		Use:   "pks",
		Short: "List available packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current repository URL
			url, err := db.GetRepoURL()
			if err != nil {
				return fmt.Errorf("failed to get repository URL: %w", err)
			}

			if url == "" {
				return fmt.Errorf("no repository set. Use 'nixtea ctx' to set a repository")
			}

			// If supervisor has no items, hydrate it first
			if !sp.HasItems() {
				if err := sp.Hydrate(url); err != nil {
					return fmt.Errorf("failed to hydrate supervisor: %w", err)
				}
			}

			// Generate and print the tree
			tree := formatPackagesTreeFromState(sp)
			nextSteps := "Next steps: ssh nixtea <pkg> <run/stop/status/logs>\n"

			// Add some spacing around the tree
			cmd.Println(titleBlock())
			cmd.Println()
			cmd.Println(tree)
			cmd.Println()
			cmd.Println(nextSteps)
			cmd.Println()

			return nil
		},
	}
}

// CreateMiddleware creates a wish middleware that handles CLI commands
func CreateMiddleware(sv *supervisor.Supervisor, cfg *config.Config, svcMngr *svc.Manager, sp *suprvisor.UnderSupervision) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			// If no command provided, continue to next middleware (TUI)
			if len(sess.Command()) == 0 {
				next(sess)
				return
			}

			ctx := context.WithValue(sess.Context(), "session", sess)

			// Initialize database
			db, err := db.New(cfg)
			if err != nil {
				fmt.Fprintf(sess.Stderr(), "Error: %v\n", err)
				_ = sess.Exit(1)
				return
			}

			// Set up root command
			rootCmd := NewRootCmd(sv, cfg, db, svcMngr, sp)
			rootCmd.SetContext(ctx)
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
