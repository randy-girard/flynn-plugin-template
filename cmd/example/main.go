package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "80"
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "flynn plugin example")
	})
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "pong")
	})
	log.Printf("example plugin listening on :%s", addr)
	log.Fatal(http.ListenAndServe(":"+addr, nil))
}
