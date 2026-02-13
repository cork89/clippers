package main

import (
	"os"
	"path/filepath"

	"github.com/cork89/clippers/internal/cli"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	godotenv.Load(filepath.Join(filepath.Dir(os.Args[0]), ".env"))

	cli.ReloadConfig()

	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
