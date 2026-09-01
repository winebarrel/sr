package main

import (
	"github.com/alecthomas/kong"
	"github.com/winebarrel/sr"
)

// version is stamped in by GoReleaser at release time.
var version string

var cli struct {
	Version kong.VersionFlag
	sr.Cmd
}

func main() {
	kctx := kong.Parse(&cli,
		kong.Name("sr"),
		kong.Description("Run a command against a different IAM Identity Center permission set."),
		kong.Vars{"version": resolveVersion(version)},
		kong.UsageOnError(),
	)

	err := cli.Run(&sr.Context{
		Exec: sr.ExecProcess,
	})

	// Only reached if the command was never started: a successful Run has
	// already replaced this process.
	kctx.FatalIfErrorf(err)
}
