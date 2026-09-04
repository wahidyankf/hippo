package main

import (
	"os"

	"github.com/wahidyankf/resource-guard/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
