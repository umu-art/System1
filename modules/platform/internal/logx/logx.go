package logx

import (
	"fmt"
	"log/slog"
	"os"
)

// New создаёт JSON-логгер в stdout, каждая запись помечена сервисом и окружением.
func New(level, service, env string) (*slog.Logger, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	return slog.New(h).With(
		slog.String("service.name", service),
		slog.String("deployment.environment.name", env),
	), nil
}
