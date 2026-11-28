//go:build integration

package main

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"system1/platform/internal/testutil"
)

func setup(t *testing.T) *echo.Echo {
	t.Helper()
	return newServer(Config{Env: "test"}, slog.New(slog.DiscardHandler), testutil.Pool(t))
}

func TestReadyz(t *testing.T) {
	e := setup(t)

	rec := testutil.Do(t, e, http.MethodGet, "/readyz", nil)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestNotImplemented(t *testing.T) {
	e := setup(t)

	rec := testutil.Do(t, e, http.MethodGet, "/api/channels-gateway/channel/list", nil)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
}
