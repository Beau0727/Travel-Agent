package main

import (
	"log/slog"
	"net/http"

	"zhilv-yuntu-go/internal/bootstrap"
	"zhilv-yuntu-go/internal/config"
	"zhilv-yuntu-go/internal/logging"
)

func main() {
	logging.Configure()

	cfg := config.Load()
	app := bootstrap.NewApp(cfg)

	addr := ":" + cfg.Port
	slog.Info("Go backend starting", "addr", "http://127.0.0.1"+addr)
	if err := http.ListenAndServe(addr, app.HTTPHandler); err != nil {
		slog.Error("Go backend stopped", "error", err)
	}
}
