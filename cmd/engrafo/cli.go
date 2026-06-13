package main

import (
	"fmt"
	"io"
)

// config holds shared state passed to all subcommands.
type config struct {
	dbPath string
	stdin  io.Reader
	stdout io.Writer
}

// runWith is the testable entry point for the CLI.
// BLOQUEANTE: stub — subcomandos pendientes de implementar en feature/cli green.
func runWith(_ []string, _ io.Reader, _ io.Writer) error {
	return fmt.Errorf("not implemented")
}
