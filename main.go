package main

import (
	"os"

	"github.com/TadahiroYamamura/hyp/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
