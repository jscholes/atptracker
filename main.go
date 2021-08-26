package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
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
	HasOverview bool
	HasLiveScores bool
	HasResults bool
	HasDraw bool
	HasSchedule bool
	HasSeedsList bool
	HasFullPlayersList bool
	HasPrizePointBreakdown bool
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.ParseFS(staticFS, "index.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template index.html: %w", err)
			return
		}

		events := GetLiveEvents()
		if err := t.Execute(w, events); err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error executing template index.html: %w", err)
			return
		}
	})

	log.Printf("Serving application on port %s; press Ctrl+C to quit", port)

	serveAddr := fmt.Sprintf(":%s", port)
	log.Fatal(http.ListenAndServe(serveAddr, r))
}

func GetLiveEvents() []LiveEvent {
	var events []LiveEvent
	events = append(events, LiveEvent{
		ID: "usopen2021",
		Year: 2021,
		Name: "US Open",
		Type: "Grand Slam",
		SinglesDrawSize: 128,
		DoublesDrawSize: 64,
		Surface: "Hard",
		HasOverview: false,
		HasLiveScores: true,
		HasResults: true,
		HasDraw: true,
		HasSchedule: true,
		HasSeedsList: false,
		HasFullPlayersList: true,
		HasPrizePointBreakdown: false,
	})
	return events
}
