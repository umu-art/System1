//go:build integration

package main

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	dialogsapi "s1_dialogs_api/models"
	"system1/platform/internal/testutil"
)

func setup(t *testing.T) (*echo.Echo, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.Pool(t)
	return newServer(Config{Env: "test"}, slog.New(slog.DiscardHandler), pool), pool
}

func TestReadyz(t *testing.T) {
	e, _ := setup(t)

	rec := testutil.Do(t, e, http.MethodGet, "/readyz", nil)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGetClient(t *testing.T) {
	e, pool := setup(t)
	serviceID := testutil.CreateService(t, pool)
	clientID := uuid.New()
	_, err := pool.Exec(t.Context(),
		`insert into dialogs.clients (id, display_name, language) values ($1, 'Клиент', 'ru')`, clientID)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(),
		`insert into dialogs.service_clients (service_id, client_id, blocked) values ($1, $2, true)`,
		serviceID, clientID)
	require.NoError(t, err)

	rec := testutil.Do(t, e, http.MethodGet,
		"/api/dialogs/client?id="+clientID.String()+"&serviceId="+serviceID.String(), nil)

	require.Equal(t, http.StatusOK, rec.Code)
	got := testutil.DecodeJSON[dialogsapi.ServiceClientDto](t, rec)
	require.Equal(t, clientID.String(), got.ClientId)
	require.Equal(t, serviceID.String(), got.ServiceId)
	require.Equal(t, "Клиент", got.DisplayName)
	require.Equal(t, "ru", got.Language)
	require.Empty(t, got.Phone)
	require.True(t, got.Blocked)
	require.False(t, got.Restricted)
}

func TestGetClientNotFound(t *testing.T) {
	e, pool := setup(t)
	serviceID := testutil.CreateService(t, pool)

	rec := testutil.Do(t, e, http.MethodGet,
		"/api/dialogs/client?id="+uuid.NewString()+"&serviceId="+serviceID.String(), nil)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetClientBadID(t *testing.T) {
	e, _ := setup(t)

	rec := testutil.Do(t, e, http.MethodGet, "/api/dialogs/client?id=abc&serviceId="+uuid.NewString(), nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNotImplemented(t *testing.T) {
	e, _ := setup(t)

	rec := testutil.Do(t, e, http.MethodGet, "/api/dialogs/dialog/list", nil)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
}
