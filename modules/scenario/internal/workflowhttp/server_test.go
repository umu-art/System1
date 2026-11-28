package workflowhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestTemporaryUserID(t *testing.T) {
	const explicitID = "19c1874e-bca1-47ab-ad78-de041c152050"
	e := echo.New()
	SetTemporaryUserID(e, func(path string) bool { return path == "/workflow" })
	e.GET("/workflow", func(c echo.Context) error {
		return c.String(http.StatusOK, TemporaryUserID(c.Request().Context()))
	})

	for _, tc := range []struct {
		name  string
		query string
		owner string
	}{
		{"omitted", "", defaultTemporaryUserID},
		{"explicit", "userId=" + explicitID, explicitID},
		{"empty", "userId=", ""},
		{"duplicate", "userId=" + explicitID + "&userId=" + explicitID, ""},
		{"invalid", "userId=0", ""},
		{"invalid encoding", "userId=%zz", ""},
		{"nil UUID", "userId=00000000-0000-0000-0000-000000000000", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/workflow?"+tc.query, nil))
			if tc.owner == "" {
				if w.Code != http.StatusBadRequest {
					t.Fatalf("expected an invalid userId to return 400, got %d", w.Code)
				}
				return
			}
			if w.Code != http.StatusOK || w.Body.String() != tc.owner {
				t.Fatalf("expected owner %q, got %d %s", tc.owner, w.Code, w.Body.String())
			}
		})
	}
}
