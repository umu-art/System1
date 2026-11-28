package httpx

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	healthzPath = "/healthz"
	readyzPath  = "/readyz"
)

// Check проверяет, что зависимость готова обслуживать запросы.
type Check func(ctx context.Context) error

type serviceInfo struct {
	Service string `json:"service"`
	Env     string `json:"env"`
}

// Health регистрирует /healthz и /readyz, /readyz отвечает 503, пока какая-то проверка не проходит.
func Health(e *echo.Echo, service, env string, checks ...Check) {
	info := serviceInfo{Service: service, Env: env}
	e.GET(healthzPath, func(c echo.Context) error {
		return c.JSON(http.StatusOK, info)
	})
	e.GET(readyzPath, func(c echo.Context) error {
		for _, check := range checks {
			if err := check(c.Request().Context()); err != nil {
				return c.JSON(http.StatusServiceUnavailable, errorBody{Message: "not ready"})
			}
		}
		return c.JSON(http.StatusOK, info)
	})
}
