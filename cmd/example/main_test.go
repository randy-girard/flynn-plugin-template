package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/randy-girard/flynn-plugin-template/internal/dashui"
)

func TestDevDashboardServesMockOverview(t *testing.T) {
	t.Setenv("DASHBOARD_SSO_OPTIONAL", "1")
	t.Setenv("DASHBOARD_DEV", "1")
	h := dashui.DevHandler(exampleMux())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard/?app_id=demo", nil))
	if rec.Code != 200 {
		t.Fatalf("overview %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Example", "internal/dashui", `href="./"`, "Metrics"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}
