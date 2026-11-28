package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	bodyLimit      = "1M"
	requestTimeout = 10 * time.Second
)

type requestIDKey struct{}

// New создаёт Echo с общими middleware и обработчиком ошибок.
func New(log *slog.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = handleError
	e.Use(
		middleware.RequestIDWithConfig(middleware.RequestIDConfig{
			RequestIDHandler: func(c echo.Context, id string) {
				ctx := context.WithValue(c.Request().Context(), requestIDKey{}, id)
				c.SetRequest(c.Request().WithContext(ctx))
			},
		}),
		requestLogger(log),
		middleware.RecoverWithConfig(middleware.RecoverConfig{
			DisableErrorHandler: true,
			LogErrorFunc: func(_ echo.Context, err error, stack []byte) error {
				return fmt.Errorf("panic: %w\n%s", err, stack)
			},
		}),
		middleware.BodyLimit(bodyLimit),
		middleware.ContextTimeout(requestTimeout),
	)
	return e
}

// RequestID возвращает X-Request-ID текущего запроса.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func requestLogger(log *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper:      skipHealth,
		HandleError:  true,
		LogLatency:   true,
		LogMethod:    true,
		LogURIPath:   true,
		LogRoutePath: true,
		LogRequestID: true,
		LogStatus:    true,
		LogError:     true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []slog.Attr{
				slog.String("http.request.method", v.Method),
				slog.String("http.route", v.RoutePath),
				slog.String("url.path", v.URIPath),
				slog.Int("http.response.status_code", v.Status),
				slog.Float64("http.server.request.duration", v.Latency.Seconds()),
				slog.String("http.request.header.x-request-id", v.RequestID),
			}
			level := slog.LevelInfo
			if v.Error != nil {
				attrs = append(attrs, slog.String("exception.message", v.Error.Error()))
			}
			if v.Status >= 500 {
				level = slog.LevelError
			}
			log.LogAttrs(c.Request().Context(), level, "request", attrs...)
			return nil
		},
	})
}

func skipHealth(c echo.Context) bool {
	p := c.Request().URL.Path
	return p == healthzPath || p == readyzPath
}
