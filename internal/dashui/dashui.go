// Package dashui is the copy-paste kit for a Flynn plugin dashboard UI.
// Serve it under the manifest dashboard.base_url path (usually /dashboard).
// Copy this directory into each plugin; do not import this template module.
package dashui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

// Session is the signed-in dashboard user scoped to one app.
type Session struct {
	UserID      string
	Email       string
	AppID       string
	AppName     string
	Plugin      string
	Permissions []string
	ResourceID  string
	Base        string
}

// Card is GET /card JSON for the Resources tab tile.
type Card struct {
	Status   string            `json:"status"`
	Attached bool              `json:"attached"`
	Summary  string            `json:"summary"`
	EnvCount int               `json:"env_count"`
	Details  map[string]string `json:"details,omitempty"`
}

// MetricEvent is one per-app datastore sample for the dashboard.
type MetricEvent struct {
	AppID      string             `json:"app_id"`
	ResourceID string             `json:"resource_id,omitempty"`
	Plugin     string             `json:"plugin"`
	Timestamp  time.Time          `json:"timestamp"`
	JobID      string             `json:"job_id,omitempty"`
	Series     map[string]float64 `json:"series"`
}

// FromRequest authenticates a dashboard SSO JWT or optional dev headers.
func FromRequest(r *http.Request) (*Session, error) {
	base := strings.TrimSpace(r.Header.Get("X-Flynn-Dashboard-Base"))
	if base == "" {
		base = "/dashboard/"
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	if optionalSSO() {
		if app := strings.TrimSpace(r.Header.Get("X-Flynn-Dashboard-App")); app != "" {
			return &Session{
				UserID: strings.TrimSpace(first(r.Header.Get("X-Flynn-Dashboard-User"), "dev")),
				Email:  strings.TrimSpace(r.Header.Get("X-Flynn-Dashboard-Email")),
				AppID:  app,
				Plugin: strings.TrimSpace(r.Header.Get("X-Flynn-Dashboard-Plugin")),
				Base:   base,
			}, nil
		}
		if app := strings.TrimSpace(r.URL.Query().Get("app_id")); app != "" {
			return &Session{UserID: "dev", AppID: app, Base: base}, nil
		}
	}
	token := bearer(r.Header.Get("Authorization"))
	if token == "" {
		return nil, fmt.Errorf("missing dashboard SSO token")
	}
	sess, err := parseJWT(token)
	if err != nil {
		return nil, err
	}
	sess.Base = base
	if sess.AppID == "" {
		sess.AppID = strings.TrimSpace(r.URL.Query().Get("app_id"))
	}
	if sess.AppID == "" {
		return nil, fmt.Errorf("SSO token missing app_id")
	}
	return sess, nil
}

func optionalSSO() bool {
	v := strings.TrimSpace(os.Getenv("DASHBOARD_SSO_OPTIONAL"))
	return v == "1" || strings.EqualFold(v, "true")
}

func bearer(h string) string {
	h = strings.TrimSpace(h)
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseJWT(raw string) (*Session, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid SSO JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid SSO JWT payload")
	}
	var claims struct {
		Aud         string   `json:"aud"`
		Exp         int64    `json:"exp"`
		Sub         string   `json:"sub"`
		Email       string   `json:"email"`
		AppID       string   `json:"app_id"`
		AppName     string   `json:"app_name"`
		Plugin      string   `json:"plugin"`
		Permissions []string `json:"permissions"`
		ResourceID  string   `json:"resource_id"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid SSO JWT claims")
	}
	if claims.Aud != "" && claims.Aud != "flynn-plugin-ui" {
		return nil, fmt.Errorf("unexpected SSO audience")
	}
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("SSO token expired")
	}
	if jwksURL := strings.TrimSpace(os.Getenv("DASHBOARD_SSO_JWKS_URL")); jwksURL != "" {
		if err := verifyES256(jwksURL, parts[0]+"."+parts[1], parts[2]); err != nil {
			return nil, err
		}
	}
	return &Session{
		UserID:      claims.Sub,
		Email:       claims.Email,
		AppID:       claims.AppID,
		AppName:     claims.AppName,
		Plugin:      claims.Plugin,
		Permissions: claims.Permissions,
		ResourceID:  claims.ResourceID,
	}, nil
}

func verifyES256(jwksURL, signingInput, sigB64 string) error {
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || len(sig) != 64 {
		return fmt.Errorf("invalid SSO signature")
	}
	req, err := http.NewRequest(http.MethodGet, jwksURL, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || len(doc.Keys) == 0 {
		return fmt.Errorf("jwks: invalid document")
	}
	key := doc.Keys[0]
	xb, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return err
	}
	yb, err := base64.RawURLEncoding.DecodeString(key.Y)
	if err != nil {
		return err
	}
	pub := ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}
	sum := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(&pub, sum[:], r, s) {
		return fmt.Errorf("SSO signature mismatch")
	}
	return nil
}

// WriteJSON writes v as application/json.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteHTML writes a small page that matches dashboard chrome inside the iframe.
func WriteHTML(w http.ResponseWriter, sess *Session, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	base := "/"
	nav := ""
	if sess != nil {
		base = html.EscapeString(sess.Base)
		nav = fmt.Sprintf(`<p class="muted">App <code>%s</code></p>`, html.EscapeString(first(sess.AppName, sess.AppID)))
	}
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html><head>
<meta charset="utf-8">
<base href="%s">
<title>%s</title>
<style>
:root { color-scheme: dark light; font-family: ui-sans-serif, system-ui, sans-serif; }
body { margin: 0; padding: 1rem 1.15rem; background: transparent; color: inherit; }
h1 { font-size: 1.05rem; margin: 0 0 .75rem; }
.muted { color: #888; font-size: .85rem; }
.card { border: 1px solid #3333; border-radius: 10px; padding: 1rem 1.1rem; margin: 0 0 1rem; }
.row { display: flex; gap: .6rem; flex-wrap: wrap; align-items: center; }
label { display: block; font-size: .8rem; margin: .4rem 0 .15rem; }
input, textarea, select { width: 100%%; max-width: 40rem; }
button, .btn { font: inherit; padding: .35rem .7rem; border-radius: 8px; cursor: pointer; }
table { width: 100%%; border-collapse: collapse; }
th, td { text-align: left; padding: .35rem .4rem; border-bottom: 1px solid #3333; font-size: .9rem; }
code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: .85rem; }
.banner { padding: .6rem .8rem; border-radius: 8px; background: #c00; color: #fff; margin-bottom: 1rem; }
.ok { color: #2a7; }
</style>
</head><body>
<h1>%s</h1>
%s
%s
</body></html>`, base, html.EscapeString(title), html.EscapeString(title), nav, body)
}

// Require is middleware that attaches *Session or writes 401.
func Require(next func(http.ResponseWriter, *http.Request, *Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := FromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next(w, r, sess)
	}
}

// PostMetrics best-effort posts a sample to the dashboard plugin-metrics webhook.
func PostMetrics(ev MetricEvent) {
	url := strings.TrimSpace(os.Getenv("DASHBOARD_METRICS_URL"))
	if url == "" {
		url = "http://dashboard.discoverd/webhooks/plugin-metrics"
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if s := strings.TrimSpace(os.Getenv("DASHBOARD_METRICS_SECRET")); s != "" {
		req.Header.Set("X-Flynn-Webhook-Secret", s)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
}
