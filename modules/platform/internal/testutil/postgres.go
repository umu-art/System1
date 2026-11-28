package testutil

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:17"

var (
	postgresOnce sync.Once
	postgresURL  string
	postgresErr  error
)

// Pool возвращает пул к Postgres в контейнере с применёнными миграциями, контейнер один на тестовый процесс.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	postgresOnce.Do(func() {
		postgresURL, postgresErr = startPostgres(context.Background())
	})
	require.NoError(t, postgresErr)

	pool, err := pgxpool.New(t.Context(), postgresURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func startPostgres(ctx context.Context) (string, error) {
	ctr, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("system1"),
		postgres.WithUsername("system1"),
		postgres.WithPassword("system1"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", fmt.Errorf("start postgres: %w", err)
	}
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("connection string: %w", err)
	}
	if err := migrate(ctx, url); err != nil {
		return "", err
	}
	return url, nil
}

func migrate(ctx context.Context, url string) error {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	dir := migrationsDir()
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		sql, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f.name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", f.name, err)
		}
	}
	return nil
}

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "postgres", "migrations")
}

type migrationFile struct {
	name    string
	version []int
}

// migrationFiles возвращает миграции Flyway в порядке версий, а не имён: V0_0_10 идёт после V0_0_2.
func migrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var files []migrationFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "V") || !strings.HasSuffix(name, ".sql") {
			continue
		}
		raw, _, ok := strings.Cut(strings.TrimPrefix(name, "V"), "__")
		if !ok {
			return nil, fmt.Errorf("migration %s: no description", name)
		}
		var version []int
		for _, part := range strings.Split(raw, "_") {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("migration %s: bad version: %w", name, err)
			}
			version = append(version, n)
		}
		files = append(files, migrationFile{name: name, version: version})
	}
	slices.SortFunc(files, func(a, b migrationFile) int {
		return cmp.Or(slices.Compare(a.version, b.version), strings.Compare(a.name, b.name))
	})
	return files, nil
}
