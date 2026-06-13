package main

import (
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}

func run(args []string) error {
	return runWith(args, os.Stdin, os.Stdout)
}
