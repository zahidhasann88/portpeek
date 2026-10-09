// Command portpeek shows which applications and containers use local network
// ports, and can stop the process holding a port.
package main

import (
	"os"

	"github.com/zahidhasann88/portpeek/internal/cli"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	build := cli.BuildInfo{Version: version, Commit: commit, Date: date}
	os.Exit(cli.Run(os.Args[1:], cli.NewDefaultDeps(build)))
}
