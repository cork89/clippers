package main

import (
	"os"

	"github.com/cork89/clippers/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
