package main

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"system1/platform/internal/httpx"
)

// newServer собирает Echo со всеми маршрутами сервиса, но не запускает его.
func newServer(cfg Config, log *slog.Logger, pool *pgxpool.Pool) *echo.Echo {
	e := httpx.New(log)
	httpx.Health(e, serviceName, cfg.Env, pool.Ping)
	registerRoutes(e, pool)
	return e
}
