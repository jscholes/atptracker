package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:embed index.html
var staticFS embed.FS

const (
	TournamentsFilename = "one-off-tournaments.json"
	HTTPClientTimeout = 30
	DesktopUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.159 Safari/537.36"
)

type TournamentDataService struct {
	http *http.Client
	providerRegistry *DataProviderRegistry
	tournaments []LiveTournament
}

func (tds *TournamentDataService) RegisterTournament(t LiveTournament) error {
	_, err := tds.providerRegistry.GetProvider(t.ProviderID)
	if err != nil {
		return err
	}

	tds.tournaments = append(tds.tournaments, t)
	return nil
}

func (tds *TournamentDataService) GetAllTournaments() []LiveTournament {
	return tds.tournaments
}

type DataProviderRegistry struct {
	providers map[string]DataProvider
}

func (dpr *DataProviderRegistry) RegisterProvider(dp DataProvider) {
	if dpr.providers == nil {
		dpr.providers = make(map[string]DataProvider)
	}

	dpr.providers[dp.ID] = dp
}

func (dpr *DataProviderRegistry) GetProvider(id string) (DataProvider, error) {
	var dp DataProvider
	dp, ok := dpr.providers[id]
	if !ok {
		return dp, fmt.Errorf("no provider registered with ID %s", id)
	}

	return dp, nil
}

type DataProvider struct {
	ID string
	BaseURL string
	UserAgent string
}

type LiveTournament struct {
	ID string
	Year int
	Name string
	Type string
	SinglesDrawSize int
	DoublesDrawSize int
	ProviderID string
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

	oneOffTournamentProviders := []DataProvider{
		DataProvider{
			ID: "gs-uso-2021",
			BaseURL: "https://www.usopen.org/en_US",
			UserAgent: DesktopUserAgent,
		},
	}

	var tournaments []LiveTournament
	tournaments, err := GetOneOffTournaments(TournamentsFilename)
	if err != nil {
		log.Printf("Error loading live tournaments: %v", err)
	}

	dpr := &DataProviderRegistry{}

	for _, p := range oneOffTournamentProviders {
		dpr.RegisterProvider(p)
	}

	dataService := TournamentDataService{
		http: &http.Client{
			Timeout: HTTPClientTimeout * time.Second,
		},
		providerRegistry: dpr,
	}

	for _, t := range tournaments {
		err := dataService.RegisterTournament(t)
		if err != nil {
			log.Printf("Error registering tournament with ID %s: %v", t.ID, err)
		}
	}

	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.ParseFS(staticFS, "index.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template index.html: %w", err)
			return
		}

		if err := t.Execute(w, dataService.GetAllTournaments()); err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error executing template index.html: %w", err)
			return
		}
	})

	log.Printf("Serving application on port %s; press Ctrl+C to quit", port)

	serveAddr := fmt.Sprintf(":%s", port)
	log.Fatal(http.ListenAndServe(serveAddr, r))
}

func GetOneOffTournaments(path string) ([]LiveTournament, error) {
	var tournaments []LiveTournament

	fileContents, err := os.ReadFile(path)
	if err != nil {
		return tournaments, fmt.Errorf("loading one-off tournaments from %s: %w", path, err)
	}

	if err := json.Unmarshal(fileContents, &tournaments); err != nil {
		return tournaments, fmt.Errorf("unmarshaling JSON from %s: %w", path, err)
	}

	return tournaments, nil
}
