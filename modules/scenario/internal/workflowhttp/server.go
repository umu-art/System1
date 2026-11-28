package workflowhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func Env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func OpenDB(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	return pgxpool.New(ctx, dsn)
}

func NewServer(pool *pgxpool.Pool, readinessQuery string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.BodyLimit("2M"))
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		var httpErr *echo.HTTPError
		switch {
		case errors.As(err, &httpErr):
			if httpErr.Internal != nil && httpErr.Code >= 500 {
				e.Logger.Error(httpErr.Internal)
			}
		default:
			e.Logger.Error(err)
		}
		e.DefaultHTTPErrorHandler(err, c)
	}
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, readinessQuery); err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable).SetInternal(err)
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	return e
}

const defaultTemporaryUserID = "00000000-0000-0000-0000-000000000001"

type temporaryUserIDKey struct{}

func SetTemporaryUserID(e *echo.Echo, applies func(path string) bool) {
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !applies(c.Path()) {
				return next(c)
			}
			query, err := url.ParseQuery(c.Request().URL.RawQuery)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "Invalid query string")
			}
			userID := defaultTemporaryUserID
			if values, ok := query["userId"]; ok {
				if len(values) != 1 {
					return echo.NewHTTPError(http.StatusBadRequest, "Provide at most one temporary userId")
				}
				userID, err = UUID(values[0], "userId")
				if err != nil {
					return err
				}
			}
			ctx := context.WithValue(c.Request().Context(), temporaryUserIDKey{}, userID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
}

func TemporaryUserID(ctx context.Context) string {
	userID, _ := ctx.Value(temporaryUserIDKey{}).(string)
	return userID
}

func Serve(e *echo.Echo) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              ":" + Env("PORT", "8080"),
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	done := make(chan error, 1)
	go func() {
		done <- server.ListenAndServe()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func ReadJSON(c echo.Context, destination any) error {
	decoder := json.NewDecoder(c.Request().Body)
	if err := decoder.Decode(destination); err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			return err
		}
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid JSON request")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return echo.NewHTTPError(http.StatusBadRequest, "Expected one JSON value")
	}
	return nil
}

func UUID(value, field string) (string, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, field+" must be a nonzero UUID")
	}
	return id.String(), nil
}
