package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/dappermint/occam/cmd"
)

// AppKit insists on the main thread, and `occam menu` runs its event loop
// there. Locking here costs nothing for the other commands.
func init() { runtime.LockOSThread() }

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "occam:", err)
		os.Exit(1)
	}
}
