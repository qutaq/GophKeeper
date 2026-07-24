package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qutaq/gophkeeper/internal/server/app"
)

var (
	version   = "dev"
	buildDate = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Fprintf(os.Stdout, "gophkeeper-server %s (%s)\n", version, buildDate)
		return
	}

	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "gophkeeper-server: %v\n", err)
		os.Exit(1)
	}
}
