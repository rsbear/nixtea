// internal/cli/cli.go

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"walross/nixtea/internal/config"
	"walross/nixtea/internal/db"
	"walross/nixtea/internal/nixapi"
	"walross/nixtea/internal/supervisor"
	"walross/nixtea/internal/svc"

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
func ctxUpdateCmd(cfg *config.Config, db *db.DB, svcMgr *svc.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update and rebuild all packages from current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create a new Nix client
			client := nixapi.NewClient()
			defer client.Close()

			// Step 1: Get current repo URL
			url, err := db.GetRepoURL()
			if err != nil {
				return fmt.Errorf("failed to get repository URL: %w", err)
			}
			if url == "" {
				return fmt.Errorf("no repository set. Use 'nixtea ctx add' to set a repository")
			}
			cmd.Printf("→ Found active repository: %s\n", url)

			// Step 2: Update flake
			// cmd.Printf("→ Updating flake...\n")
			// if err := client.UpdateFlake(url); err != nil {
			// 	return fmt.Errorf("failed to update flake: %w", err)
			// }
			// cmd.Printf("✓ Flake updated\n")

			// Step 3: Get all available packages
			packages, err := client.GetSystemPackages(url)
			if err != nil {
				return fmt.Errorf("step 3: failed to get packages: %w", err)
			}
			cmd.Printf("→ Found %d packages\n", len(packages))

			// Step 4: Build and install each package
			for key, pkg := range packages {
				cmd.Printf("\nProcessing package: %s (%s)\n", pkg.Name, key)

				// Check if service exists and get its status
				status, err := svcMgr.Status(key)
				wasRunning := err == nil && status == service.StatusRunning

				if err == nil {
					// Service exists, we need to stop it first
					cmd.Printf("  → Stopping existing service...\n")
					if err := svcMgr.Stop(key); err != nil {
						cmd.Printf("  ! Warning: Failed to stop service: %v\n", err)
					}
				}

				// Build the package
				buildResult, err := client.BuildPackage(url, key)
				if err != nil {
					cmd.Printf("  ✗ Build failed: %v\n", err)
					continue
				}
				cmd.Printf("  ✓ Built successfully: %s\n", buildResult.StorePath)

				// Install service
				cmd.Printf("  → Installing service...\n")
				if err := svcMgr.Install(key, buildResult.BinaryPath); err != nil {
					cmd.Printf("  ✗ Failed to install service: %v\n", err)
					continue
				}
				cmd.Printf("  ✓ Service installed\n")

				// Restart service if it was running before
				if wasRunning {
					cmd.Printf("  → Restarting service...\n")
					if err := svcMgr.Start(key); err != nil {
						cmd.Printf("  ✗ Failed to restart service: %v\n", err)
						continue
					}
					cmd.Printf("  ✓ Service restarted\n")
				}
			}

			cmd.Println("\n✓ Update complete!")
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

func NewRootCmd(sv *supervisor.Supervisor, cfg *config.Config, db *db.DB, svcMgr *svc.Manager) *cobra.Command {
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
	ctxCmd.AddCommand(ctxAddCmd(cfg, db), ctxUpdateCmd(cfg, db, svcMgr))

	// pks - list packages
	pksCmd := &cobra.Command{
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

			// Create a new nixapi client
			client := nixapi.NewClient()

			// Generate and print the tree
			tree, err := formatPackagesTree(client, url)
			if err != nil {
				return err
			}

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

	// Add all commands to root
	rootCmd.AddCommand(ctxCmd)
	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(pksCmd)
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

// CreateMiddleware creates a wish middleware that handles CLI commands
func CreateMiddleware(sv *supervisor.Supervisor, cfg *config.Config, svcMngr *svc.Manager) wish.Middleware {
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
			rootCmd := NewRootCmd(sv, cfg, db, svcMngr)
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
