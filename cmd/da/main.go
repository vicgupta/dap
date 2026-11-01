package main

import (
	"os"

	"github.com/vicgupta/dap/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Options{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}
