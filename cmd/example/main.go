package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/randy-girard/flynn-plugin-template/internal/dashui"
)

func main() {
	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "80"
	}
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
	mux.HandleFunc("GET /dashboard", dashui.Require(func(w http.ResponseWriter, r *http.Request, sess *dashui.Session) {
		dashui.WriteHTML(w, sess, "Example", `<div class="card"><p>Template plugin UI. Copy <code>internal/dashui</code> into a real plugin.</p></div>`)
	}))
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
		dashui.WriteHTML(w, sess, "Metrics", `<div class="card"><p class="muted">No samples yet. Plugins POST series to the dashboard plugin-metrics webhook.</p></div>`)
	}))
	log.Printf("example plugin listening on :%s", addr)
	log.Fatal(http.ListenAndServe(":"+addr, mux))
}
