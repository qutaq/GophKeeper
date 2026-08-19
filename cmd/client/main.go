package main

import (
	"fmt"
	"os"

	"github.com/qutaq/gophkeeper/internal/client/cli"
)

var (
	version   = "dev"
	buildDate = "unknown"
)

func main() {
	root := cli.NewRootCommand(cli.Options{
		Version:   version,
		BuildDate: buildDate,
	})
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "gophkeeper-client: %v\n", err)
		os.Exit(1)
	}
}
