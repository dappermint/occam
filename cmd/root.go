// Package cmd wires the occam command tree.
package cmd

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleDim   = lipgloss.NewStyle().Faint(true)
	styleHit   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	styleKey   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "occam",
		Short:         "control a Razer BlackShark V3 Pro from macOS, without Synapse",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newProbe())
	root.AddCommand(newConsole())
	root.AddCommand(newEQ())
	root.AddCommand(newGet())
	root.AddCommand(newListen())
	root.AddCommand(newProfile())
	return root
}

// Execute runs the command tree and returns the first error.
func Execute() error {
	return newRoot().Execute()
}
