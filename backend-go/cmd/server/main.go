package main

import (
	"fmt"
	"log"
	"net/http"

	"travel-agent-go/internal/bootstrap"
	"travel-agent-go/internal/config"
	"travel-agent-go/internal/logging"
)

func main() {
	logging.Configure()

	cfg := config.Load()
	app := bootstrap.NewApp(cfg)
	addr := ":" + cfg.Port

	logging.Info(nil, "travel agent backend starting", "addr", addr)
	if err := http.ListenAndServe(addr, app.HTTPHandler); err != nil {
		log.Fatal(fmt.Errorf("listen on %s: %w", addr, err))
	}
}
