package main

import (
	"os"

	"github.com/harzva/aliyunpan-cli/internal/app"
)

func main() {
	a := app.New(os.Stdin, os.Stdout, os.Stderr)
	if err := a.Run(os.Args[1:]); err != nil {
		a.PrintError(err)
		os.Exit(app.ExitCode(err))
	}
}
