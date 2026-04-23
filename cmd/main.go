package main

import (
	"os"

	"github.com/ashraf/log-serve/internal/server"
)

func main() {
	os.Exit(server.RunCLI(os.Args[1:]))
}
