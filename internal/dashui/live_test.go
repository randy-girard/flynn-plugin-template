package dashui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyLivePostsAppKindWithoutTenant(t *testing.T) {
	var got map[string]string
	var secret, path string
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		secret = r.Header.Get("X-Flynn-Webhook-Secret")
		path = r.URL.Path
		if r.URL.RawQuery != "" {
			t.Errorf("secret leaked into query: %s", r.URL.RawQuery)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("DASHBOARD_METRICS_URL", srv.URL+"/webhooks/plugin-metrics")
	t.Setenv("DASHBOARD_METRICS_SECRET", "sekrit")
	NotifyLive("template", "shop", "resources", "orders")
	NotifyLive("template", "", "resources", "dropped")
	NotifyLive("", "shop", "resources", "dropped")
	NotifyLive("template", "tenant:acme", "resources", "dropped")
	NotifyLive("template", "shop/secret", "resources", "dropped")

	if calls != 1 {
		t.Fatalf("posts = %d", calls)
	}
	if secret != "sekrit" {
		t.Fatalf("secret header = %q", secret)
	}
	if path != "/webhooks/live" {
		t.Fatalf("path = %s", path)
	}
	if got["app_id"] != "shop" || got["plugin"] != "template" || got["kind"] != "resources" || got["id"] != "orders" {
		t.Fatalf("body = %#v", got)
	}
	if _, ok := got["tenant"]; ok {
		t.Fatal("tenant field published")
	}
	if _, ok := got["topic"]; ok {
		t.Fatal("client topic published")
	}
}
