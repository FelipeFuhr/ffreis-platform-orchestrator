package main

import (
	"os"

	"github.com/ffreis/platform-orchestrator/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
