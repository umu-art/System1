package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"s1_dialogs_api/handlers"
	"s1_dialogs_api/models"
)

const serviceName = "s1-dialogs"

type handler struct {
	env string
}

func (h *handler) GetDialogsInfo(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, models.ServiceInfoDto{Service: serviceName, Env: h.env})
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer logger.Sync()

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.GET("/healthz", health)
	e.GET("/readyz", health)

	handlers.RouteDialogsAPI(e, &handler{env: getenv("S1_ENV", "local")})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	port := getenv("PORT", "8080")
	go func() {
		logger.Info("starting server", zap.String("service", serviceName), zap.String("port", port))
		if err := e.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("echo start", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Error("echo shutdown", zap.Error(err))
	}
}

func health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
