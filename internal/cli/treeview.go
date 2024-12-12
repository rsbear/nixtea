// file: internal/cli/treeview.go

package cli

import (
	"fmt"
	"sort"
	"walross/nixtea/internal/nixapi"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
)

var (
	enumeratorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")).
			MarginRight(0)

	rootStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("35"))

	itmStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212"))

	hashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))
)

func formatPackagesTree(client *nixapi.Client, repoURL string) (string, error) {
	packages, err := client.GetSystemPackages(repoURL)
	if err != nil {
		return "", fmt.Errorf("failed to get packages: %w", err)
	}

	// Sort packages by name for consistent display
	pkgKeys := make([]string, 0, len(packages))
	for key := range packages {
		pkgKeys = append(pkgKeys, key)
	}
	sort.Strings(pkgKeys)

	// Create package entries with styled names and hashes
	items := make([]any, len(pkgKeys))
	for i, key := range pkgKeys {
		pkg := packages[key]
		items[i] = fmt.Sprintf("%s %s",
			pkg.Name,
			hashStyle.Render("#"+key),
		)
	}

	// Build the tree
	t := tree.Root("⚡ Nixtea Packages").
		Child(items...). // Pass all entries as variadic arguments
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(enumeratorStyle).
		RootStyle(rootStyle).
		ItemStyle(itmStyle)

	// t.Root(repoURL)

	return t.String(), nil
}
