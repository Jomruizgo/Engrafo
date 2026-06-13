package main

import "os"

func main() {
	if err := run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}

// run is the entry point for the CLI; implemented in feature/cli.
func run(_ []string) error {
	return nil
}
