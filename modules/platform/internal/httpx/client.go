package httpx

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// NewClient создаёт HTTP-клиент для вызовов других сервисов, он передаёт X-Request-ID из контекста.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: requestIDTransport{base: http.DefaultTransport},
	}
}

type requestIDTransport struct {
	base http.RoundTripper
}

func (t requestIDTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if id := RequestID(r.Context()); id != "" {
		r = r.Clone(r.Context())
		r.Header.Set(echo.HeaderXRequestID, id)
	}
	return t.base.RoundTrip(r)
}
