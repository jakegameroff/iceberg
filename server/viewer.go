package main

import "net/http"

// serveView serves the static viewer page. It does NOT render content — the
// page fetches /<topic> and renders it client-side in sandboxed elements.
func serveView(w http.ResponseWriter) {
	page, err := staticFS.ReadFile("static/iceberg.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(page)
}
