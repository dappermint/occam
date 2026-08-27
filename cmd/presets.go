package cmd

import (
	"fmt"
	"strings"

	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

func newPresets() *cobra.Command {
	var footstep bool

	c := &cobra.Command{
		Use:   "presets [filter]",
		Short: "list Razer's EQ library",
		Args:  cobra.MaximumNArgs(1),
		Long: "Every preset Synapse offers, with the curve it writes. Pass any of\n" +
			"these names to `occam eq --preset`.",
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := ""
			if len(args) == 1 {
				filter = strings.ToLower(args[0])
			}

			shown := 0
			for _, e := range proto.Library {
				if filter != "" && !strings.Contains(strings.ToLower(e.Name), filter) {
					continue
				}
				curve := e.Bands
				if footstep {
					curve = e.Footstep
				}
				fmt.Printf("  %s  %s  %s\n",
					styleKey.Render(fmt.Sprintf("%3d", e.ID)),
					styleTitle.Render(fmt.Sprintf("%-36s", e.Name)),
					curve)
				shown++
			}
			if shown == 0 {
				return fmt.Errorf("nothing in the library matches %q", filter)
			}
			fmt.Printf("\n  %s %d of %d\n",
				styleDim.Render("shown"), shown, len(proto.Library))
			return nil
		},
	}
	c.Flags().BoolVar(&footstep, "footstep", false, "show the footstep scaling curve instead of the bands")
	return c
}
