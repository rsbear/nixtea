// internal/cli/cli.go

package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"walross/nixtea/internal/config"
	"walross/nixtea/internal/db"
	"walross/nixtea/internal/suprvisor"

	"github.com/charmbracelet/lipgloss/tree"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"

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
func ctxUpdateCmd(db *db.DB, sp *suprvisor.UnderSupervision) *cobra.Command {
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

			err = sp.HydrateWithTimeout(url, 5*time.Minute)
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
func ctxAddCmd(db *db.DB) *cobra.Command {
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
				if err := sp.HydrateWithTimeout(url, 5*time.Minute); err != nil {
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

func pksStopCmd(sp *suprvisor.UnderSupervision, db *db.DB) *cobra.Command {
	return &cobra.Command{
		Use:   "stop [package]",
		Short: "Stop a running package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]

			// Get current repository URL
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
				if err := sp.HydrateWithTimeout(url, 5*time.Minute); err != nil {
					return fmt.Errorf("failed to hydrate supervisor: %w", err)
				}
			}

			// Attempt to stop the package
			cmd.Printf("→ Stopping package %s...\n", pkgKey)
			if err := sp.Stop(pkgKey); err != nil {
				cmd.Printf("✗ Failed to stop package: %v\n", err)
				return nil // Return nil to avoid double error message
			}

			cmd.Printf("✓ Package %s stopped successfully\n\n", pkgKey)
			cmd.Printf("To check package status:\n")
			cmd.Printf("  nixtea pks status %s\n", pkgKey)

			return nil
		},
	}
}

func NewRootCmd(cfg *config.Config, db *db.DB, sp *suprvisor.UnderSupervision) *cobra.Command {
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
	ctxCmd.AddCommand(ctxAddCmd(db), ctxUpdateCmd(db, sp))

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

	pksCmd := &cobra.Command{
		Use:   "pks",
		Short: "Package management commands",
		RunE:  pksListCmd(cfg, db, sp).RunE,
	}

	// Add this to NewRootCmd, alongside the other commands
	pksStatusCmd := &cobra.Command{
		Use:   "status [package]",
		Short: "Show status of running packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current repository URL
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
				if err := sp.HydrateWithTimeout(url, 5*time.Minute); err != nil {
					return fmt.Errorf("failed to hydrate supervisor: %w", err)
				}
			}

			items := sp.GetSupervised()

			// If no packages are being supervised
			if len(items) == 0 {
				cmd.Println("No packages are currently being supervised.")
				return nil
			}

			// If a specific package is requested
			if len(args) > 0 {
				pkgKey := args[0]
				item, exists := items[pkgKey]
				if !exists {
					return fmt.Errorf("package %s not found", pkgKey)
				}

				// Print detailed status for the specific package
				status := formatPackageStatus(pkgKey, item)

				// Add a header
				headerStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("99")).
					Bold(true).
					PaddingBottom(1)

				cmd.Printf("%s\n%s",
					headerStyle.Render("Package Status"),
					status)
				return nil
			}

			// Otherwise, show status of all packages
			headerStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("99")).
				Bold(true).
				PaddingBottom(1)

			cmd.Println(headerStyle.Render("Package Status Overview"))
			cmd.Println()

			// Create a tabwriter for aligned output
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PACKAGE\tSTATUS\tPID\n")

			// Sort keys for consistent output
			var keys []string
			for k := range items {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			// Print each package's status
			for _, key := range keys {
				item := items[key]

				// Style the status
				var statusStyle lipgloss.Style
				switch item.Status {
				case "running":
					statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
				case "stopped":
					statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
				default:
					statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
				}

				pid := "-"
				if item.Pid > 0 {
					pid = fmt.Sprintf("%d", item.Pid)
				}

				fmt.Fprintf(w, "%s\t%s\t%s\n",
					item.Name,
					statusStyle.Render(item.Status),
					pid)
			}
			w.Flush()

			// Add help text at the bottom
			cmd.Println("\nFor detailed status of a specific package:")
			cmd.Printf("  nixtea pks status <package>\n")

			return nil
		},
	}

	pksLogsCmd := &cobra.Command{
		Use:   "logs [package]",
		Short: "Stream logs from a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgKey := args[0]

			// Get current repository URL
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
				if err := sp.HydrateWithTimeout(url, 5*time.Minute); err != nil {
					return fmt.Errorf("failed to hydrate supervisor: %w", err)
				}
			}

			// Get the output stream
			output, err := sp.StreamOutput(pkgKey)
			if err != nil {
				return err
			}

			// Copy the output stream to stdout
			_, err = io.Copy(cmd.OutOrStdout(), output)
			return err

		},
	}

	pksCmd.AddCommand(pksRunCmd(cfg, db, sp), pksStatusCmd, pksStopCmd(sp, db), pksLogsCmd)

	// Add all commands to root
	rootCmd.AddCommand(ctxCmd)
	rootCmd.AddCommand(pksCmd)
	rootCmd.AddCommand(helpCmd)

	return rootCmd
}

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

// pksListCmd creates the 'pks' command
func pksListCmd(cfg *config.Config, db *db.DB, sp *suprvisor.UnderSupervision) *cobra.Command {
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
				if err := sp.HydrateWithTimeout(url, 5*time.Minute); err != nil {
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

// Add this helper function to format package status
func formatPackageStatus(name string, state suprvisor.ItemState) string {
	var status strings.Builder
	w := tabwriter.NewWriter(&status, 0, 0, 2, ' ', 0)

	// Format status with color
	var statusStyle lipgloss.Style
	switch state.Status {
	case "running":
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green
	case "stopped":
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red
	default:
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")) // gray
	}

	fmt.Fprintf(w, "Name:\t%s\n", name)
	fmt.Fprintf(w, "Status:\t%s\n", statusStyle.Render(state.Status))
	if state.Pid > 0 {
		fmt.Fprintf(w, "PID:\t%d\n", state.Pid)
	}
	if state.BinaryPath != "" {
		fmt.Fprintf(w, "Binary:\t%s\n", state.BinaryPath)
	}
	w.Flush()
	return status.String()
}

// CreateMiddleware creates a wish middleware that handles CLI commands
func CreateMiddleware(cfg *config.Config, sp *suprvisor.UnderSupervision) wish.Middleware {
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
			rootCmd := NewRootCmd(cfg, db, sp)
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
