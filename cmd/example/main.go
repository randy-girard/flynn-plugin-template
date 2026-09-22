package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/randy-girard/flynn-plugin-template/internal/dashui"
)

var exampleDashNav = [][2]string{
	{"./", "Overview"},
	{"metrics", "Metrics"},
}

func writeDash(w http.ResponseWriter, sess *dashui.Session, title, body string) {
	dashui.WriteHTML(w, sess, title, dashui.Nav(sess, exampleDashNav...)+body)
}

func exampleMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, "flynn plugin example")
	})
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "pong")
	})
	overview := dashui.Require(func(w http.ResponseWriter, r *http.Request, sess *dashui.Session) {
		writeDash(w, sess, "Example", `<div class="card"><p>Template plugin UI. Copy <code>internal/dashui</code> into a real plugin.</p></div>`)
	})
	mux.HandleFunc("GET /dashboard", overview)
	mux.HandleFunc("GET /dashboard/{$}", overview)
	mux.HandleFunc("GET /dashboard/card", dashui.Require(func(w http.ResponseWriter, r *http.Request, sess *dashui.Session) {
		dashui.WriteJSON(w, http.StatusOK, dashui.Card{
			Status:   "ready",
			Attached: true,
			Summary:  "example plugin",
			EnvCount: 0,
			Details:  map[string]string{"app": sess.AppID},
		})
	}))
	mux.HandleFunc("GET /dashboard/metrics", dashui.Require(func(w http.ResponseWriter, r *http.Request, sess *dashui.Session) {
		writeDash(w, sess, "Metrics", `<div class="card"><p class="muted">No samples yet. Plugins POST series to the dashboard plugin-metrics webhook.</p></div>`)
	}))
	return mux
}

func main() {
	h := exampleMux()
	listen := ":" + strings.TrimSpace(os.Getenv("PORT"))
	if listen == ":" {
		listen = ":80"
	}
	if dashui.DevEnabled() {
		_ = os.Setenv("DASHBOARD_SSO_OPTIONAL", "1")
		h = dashui.DevHandler(h)
		listen = dashui.DevAddr()
		log.Printf("example dashboard-dev listening on %s — open http://127.0.0.1%s/dashboard/?app_id=demo", listen, listen)
	} else {
		log.Printf("example plugin listening on %s", listen)
	}
	log.Fatal(http.ListenAndServe(listen, h))
}
