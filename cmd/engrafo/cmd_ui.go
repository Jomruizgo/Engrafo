package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"

	"github.com/Jomruizgo/Engrafo/v2/internal/graph"
	"github.com/Jomruizgo/Engrafo/v2/internal/ui"
)

func cmdUI(cfg *config, args []string) error {
	fs2 := flag.NewFlagSet("ui", flag.ContinueOnError)
	port := fs2.Int("port", 8080, "port to listen on (localhost only)")
	fs2.SetOutput(cfg.stdout)
	if err := fs2.Parse(args); err != nil {
		return err
	}

	dbPath, err := cfg.resolveDB()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}
	s, err := graph.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer s.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	srv := ui.NewServer(s)
	fmt.Fprintf(cfg.stdout, "engrafo ui â†’ http://%s\n", addr)
	return http.Serve(ln, srv.Handler())
}
