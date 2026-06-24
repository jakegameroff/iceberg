package main

import (
	"embed"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

var topicRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

const maxBody = 25 << 20 // 25 MiB

func main() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		if !rateLimit(w, r) {
			return
		}

		// install script + binary downloads, served from ./dist on disk
		if r.Method == http.MethodGet {
			if r.URL.Path == "/install.sh" {
				http.ServeFile(w, r, "dist/install.sh")
				return
			}
			if strings.HasPrefix(r.URL.Path, "/dl/") {
				http.StripPrefix("/dl/", http.FileServer(http.Dir("dist/dl"))).ServeHTTP(w, r)
				return
			}
			// viewer page for a topic: /v/<topic> — static page, renders client-side
			if strings.HasPrefix(r.URL.Path, "/v/") {
				serveView(w)
				return
			}
		}

		if r.URL.Path == "/" {
			page, err := staticFS.ReadFile("static/index.html")
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.Write(page)
		} else {

			topicName := strings.TrimPrefix(r.URL.Path, "/")

			if !topicRe.MatchString(topicName) {
				http.Error(w, "bad topic [accepts lowercase letters, digits, or hyphens]", http.StatusBadRequest)
				return
			}

			switch r.Method {
			case http.MethodGet:
				// read the topic, write its body back
				body := readStore(topicName)
				if body == nil {
					http.Error(w, "topic not found", http.StatusNotFound)
				} else {
					w.Write(body)
				}

			case http.MethodPost:
				// read r.Body (capped), store under the topic
				r.Body = http.MaxBytesReader(w, r.Body, maxBody)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
					return
				}
				writeStore(topicName, body)
				w.WriteHeader(http.StatusCreated)

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}

	})

	startCleaner(ttl)
	startLimiterCleaner(time.Minute)

	log.Fatal(http.ListenAndServe(":5555", nil))
}
