//go:build !windows

package agentsync

import "fmt"

// createJunction is unavailable off Windows; junctions apply only on Windows
// and only to directories.
func createJunction(_, _ string) error {
	return fmt.Errorf("junctions supported only on Windows")
}
