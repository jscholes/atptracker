package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:embed *.html
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

func (tds *TournamentDataService) GetTournament(id string) (LiveTournament, error) {
	var tournament LiveTournament
	ok := false

	for _, t := range tds.GetAllTournaments() {
		if t.ID == id {
			tournament = t
			ok = true
			break
		}
	}

	if !ok {
		return tournament, fmt.Errorf("no tournament registered with ID %s", id)
	}

	return tournament, nil
}

func (tds *TournamentDataService) GetPlayers(t LiveTournament) ([]Player, error) {
	var players []Player

	provider, err := tds.providerRegistry.GetProvider(t.ProviderID)
	if err != nil {
		return players, err
	}

	url, err := provider.PlayersURL(t)
	if err != nil {
		return players, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return players, err
	}

	req.Header.Set("User-Agent", provider.UserAgent())

	resp, err := tds.http.Do(req)
	if err != nil {
		return players, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return players, err
	}

	if resp.StatusCode != http.StatusOK {
		return players, fmt.Errorf("HTTP/%s\n%s", resp.Status, body)
	}

	return provider.DeserializePlayers(body)
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

	dpr.providers[dp.ID()] = dp
}

func (dpr *DataProviderRegistry) GetProvider(id string) (DataProvider, error) {
	var dp DataProvider
	dp, ok := dpr.providers[id]
	if !ok {
		return dp, fmt.Errorf("no provider registered with ID %s", id)
	}

	return dp, nil
}

type DataProvider interface {
	ID() string
	BaseURL() string
	UserAgent() string
	PlayersURL(t LiveTournament) (string, error)
	DeserializePlayers(data []byte) ([]Player, error)
}

type USOpenProvider struct {}

func (u USOpenProvider) ID() string {
	return "gs-uso"
}

func (u USOpenProvider) BaseURL() string {
	return "https://www.usopen.org/en_US"
}

func (u USOpenProvider) UserAgent() string {
	return DesktopUserAgent
}

func (u USOpenProvider) PlayersURL(t LiveTournament) (string, error) {
	return fmt.Sprintf("%s/scores/feeds/%d/players/players.json", u.BaseURL(), t.Year), nil
}

func (u USOpenProvider) DeserializePlayers(data []byte) ([]Player, error) {
	type USOpenPlayer struct {
		FirstName string `json:"first_name"`
		LastName string `json:"last_name"`
	}

	type USOpenPlayerList struct {
		Players []USOpenPlayer
	}

	var players []Player
	var USOpenPlayers USOpenPlayerList

	if err := json.Unmarshal(data, &USOpenPlayers); err != nil {
		return players, fmt.Errorf("unmarshaling JSON response: %w", err)
	}

	for _, p := range USOpenPlayers.Players {
		players = append(players, Player{
			Name: fmt.Sprintf("%s %s", p.FirstName, p.LastName),
		})
	}

	return players, nil
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

type Player struct {
	Name string
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	oneOffTournamentProviders := []DataProvider{
		USOpenProvider{},
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
		t, err := template.New("index.html").ParseFS(staticFS, "index.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template: %v", err)
			return
		}

		t, err = t.ParseFS(staticFS, "layout.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template: %v", err)
			return
		}

		if err := t.ExecuteTemplate(w, "index.html", dataService.GetAllTournaments()); err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error executing template index.html: %v", err)
			return
		}
	})

	r.Get("/tournament/{id}/{year}/players", func(w http.ResponseWriter, r *http.Request) {
		tournamentID := chi.URLParam(r, "id")
		tournament, err := dataService.GetTournament(tournamentID)
		if err != nil {
			http.Error(w, "404 tournament not found", http.StatusNotFound)
			log.Printf("Error fetching player list for tournament with ID %s: %v", tournamentID, err)
			return
		}

		players, err := dataService.GetPlayers(tournament)
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error fetching player list for tournament with ID %s: %v", tournamentID, err)
			return
		}

		t, err := template.New("players.html").ParseFS(staticFS, "players.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template: %v", err)
			return
		}

		t, err = t.ParseFS(staticFS, "layout.html")
		if err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error loading template: %v", err)
			return
		}

		if err := t.ExecuteTemplate(w, "players.html", players); err != nil {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			log.Printf("Error executing template players.html: %v", err)
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
