package main

import (
	"fmt"
	"os"

	"github.com/dappermint/occam/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "occam:", err)
		os.Exit(1)
	}
}
