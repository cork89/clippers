package main

import (
	"os"

	"github.com/cork89/clippers/internal/cli"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
