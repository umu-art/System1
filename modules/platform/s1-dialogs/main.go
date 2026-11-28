package main

import (
	"log/slog"
	"os"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("service stopped",
			slog.String("service.name", serviceName),
			slog.String("exception.message", err.Error()),
		)
		os.Exit(1)
	}
}
