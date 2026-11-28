package testutil

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// CreateService создаёт владельца и его сервис напрямую в базе и возвращает id сервиса.
func CreateService(t testing.TB, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ownerID, serviceID := uuid.New(), uuid.New()
	_, err := pool.Exec(t.Context(),
		`insert into owners.owners (id, username, first_name, last_name, email, password_hash)
		 values ($1, $2, 'Тест', 'Владелец', $3, 'hash')`,
		ownerID, "owner-"+ownerID.String(), ownerID.String()+"@example.com")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(),
		`insert into owners.services (id, owner_id, name) values ($1, $2, 'Тестовый сервис')`,
		serviceID, ownerID)
	require.NoError(t, err)
	return serviceID
}
