package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"system1/platform/internal/apperr"
	"system1/platform/internal/testutil"
)

func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"invalid", apperr.Invalid("id must be a uuid"), http.StatusBadRequest, "id must be a uuid"},
		{"unauthenticated", apperr.Unauthenticated("no token"), http.StatusUnauthorized, "no token"},
		{"forbidden", apperr.Forbidden("not yours"), http.StatusForbidden, "not yours"},
		{"not found", apperr.NotFound("client not found"), http.StatusNotFound, "client not found"},
		{"conflict", apperr.Conflict("already exists"), http.StatusConflict, "already exists"},
		{"not implemented", apperr.NotImplemented(), http.StatusNotImplemented, internalError},
		{"wrapped apperr", errorsJoin(apperr.NotFound("gone")), http.StatusNotFound, "gone"},
		{"echo error", echo.NewHTTPError(http.StatusMethodNotAllowed, "nope"), http.StatusMethodNotAllowed, "nope"},
		{"plain error", errors.New("connection refused to 10.0.0.1"), http.StatusInternalServerError, internalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(slog.New(slog.DiscardHandler))
			e.GET("/x", func(echo.Context) error { return tt.err })

			rec := testutil.Do(t, e, http.MethodGet, "/x", nil)

			require.Equal(t, tt.status, rec.Code)
			require.Equal(t, tt.message, testutil.DecodeJSON[errorBody](t, rec).Message)
		})
	}
}

func TestPanicBecomesInternalError(t *testing.T) {
	e := New(slog.New(slog.DiscardHandler))
	e.GET("/x", func(echo.Context) error { panic("boom") })

	rec := testutil.Do(t, e, http.MethodGet, "/x", nil)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, internalError, testutil.DecodeJSON[errorBody](t, rec).Message)
}

func TestRequestIDInContext(t *testing.T) {
	e := New(slog.New(slog.DiscardHandler))
	var got string
	e.GET("/x", func(c echo.Context) error {
		got = RequestID(c.Request().Context())
		return c.NoContent(http.StatusNoContent)
	})

	rec := testutil.Do(t, e, http.MethodGet, "/x", nil)

	require.NotEmpty(t, got)
	require.Equal(t, got, rec.Header().Get(echo.HeaderXRequestID))
}

func errorsJoin(err error) error {
	return errors.Join(errors.New("context"), err)
}
