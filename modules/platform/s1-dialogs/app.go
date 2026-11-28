package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"golang.org/x/sync/errgroup"

	"system1/platform/internal/httpx"
	"system1/platform/internal/logx"
	"system1/platform/internal/pg"
)

const serviceName = "s1-dialogs"

// run собирает зависимости, запускает сервис и останавливает его по сигналу.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log, err := logx.New(cfg.LogLevel, serviceName, cfg.Env)
	if err != nil {
		return err
	}
	pool, err := pg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	e := newServer(cfg, log, pool)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return httpx.Serve(ctx, e, cfg.Port)
	})
	log.Info("service started", "server.port", cfg.Port)

	if err := g.Wait(); err != nil {
		return err
	}
	log.Info("service stopped")
	return nil
}
