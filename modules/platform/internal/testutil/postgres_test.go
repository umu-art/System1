package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationFilesSortedByVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"V0_0_10__dialogs_add.sql",
		"V0_0_2__dialogs_init.sql",
		"V0_1_0__owners_add.sql",
		"V0_0_1__owners_init.sql",
		"README.md",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}

	files, err := migrationFiles(dir)
	require.NoError(t, err)

	var names []string
	for _, f := range files {
		names = append(names, f.name)
	}
	require.Equal(t, []string{
		"V0_0_1__owners_init.sql",
		"V0_0_2__dialogs_init.sql",
		"V0_0_10__dialogs_add.sql",
		"V0_1_0__owners_add.sql",
	}, names)
}
