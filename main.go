package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

//go:embed index.html
var staticFS embed.FS

const (
	TournamentsFilename = "one-off-tournaments.json"
)

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

		events, err := GetOneOffEvents(TournamentsFilename)
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading live events: %v", err)
			return
		}

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

func GetOneOffEvents(path string) ([]LiveEvent, error) {
	var events []LiveEvent

	fileContents, err := os.ReadFile(path)
	if err != nil {
		return events, fmt.Errorf("loading one-off tournaments from %s: %w", path, err)
	}

	if err := json.Unmarshal(fileContents, &events); err != nil {
		return events, fmt.Errorf("unmarshaling JSON from %s: %w", path, err)
	}

	return events, nil
}
