package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

//go:embed index.html
var staticFS embed.FS

type LiveEvent struct {
	ID string
	Year int
	Name string
	Type string
	SinglesDrawSize int
	DoublesDrawSize int
	Surface string
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.ParseFS(staticFS, "index.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template index.html: %w", err)
			return
		}

		events := GetSampleLiveEvents()
		if err := t.Execute(w, events); err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error executing template index.html: %w", err)
			return
		}
	})

	log.Printf("Serving application on port %s; press Ctrl+C to quit", port)

	serveAddr := fmt.Sprintf(":%s", port)
	log.Fatal(http.ListenAndServe(serveAddr, nil))
}

func GetSampleLiveEvents() []LiveEvent {
	var events []LiveEvent
	events = append(events, LiveEvent{"416", 2021, "Rome", "1000", 56, 32, "Clay"})
	events = append(events, LiveEvent{"460", 2021, "Heilbronn", "Challenger", 32, 16, "Clay"})
	events = append(events, LiveEvent{"7694", 2021, "Lyon", "250", 28, 16, "Clay"})
	return events
}
