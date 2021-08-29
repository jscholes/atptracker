package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
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

type App struct {
	DataService *TournamentDataService
	StaticFiles fs.FS
}

func (a *App) currentTournaments(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("index.html").ParseFS(a.StaticFiles, "index.html")
	if err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error loading template: %v", err)
		return
	}

	t, err = t.ParseFS(a.StaticFiles, "layout.html")
	if err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error loading template: %v", err)
		return
	}

	if err := t.ExecuteTemplate(w, "index.html", a.DataService.GetAllTournaments()); err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error executing template index.html: %v", err)
		return
	}
}

func (a *App) tournamentPlayers(w http.ResponseWriter, r *http.Request) {
	tournamentID := chi.URLParam(r, "id")
	tournament, err := a.DataService.GetTournament(tournamentID)
	if err != nil {
		http.Error(w, "404 tournament not found", http.StatusNotFound)
		log.Printf("Error fetching player list for tournament with ID %s: %v", tournamentID, err)
		return
	}

	players, err := a.DataService.GetPlayers(tournament)
	if err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error fetching player list for tournament with ID %s: %v", tournamentID, err)
		return
	}

	t, err := template.New("players.html").ParseFS(a.StaticFiles, "players.html")
	if err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error loading template: %v", err)
		return
	}

	t, err = t.ParseFS(a.StaticFiles, "layout.html")
	if err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error loading template: %v", err)
		return
	}

	ctx := struct{
		Tournament LiveTournament
		Players []Event
	}{
		Tournament: tournament,
		Players: players,
	}

	if err := t.ExecuteTemplate(w, "players.html", ctx); err != nil {
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		log.Printf("Error executing template players.html: %v", err)
		return
	}
}

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

func (tds *TournamentDataService) GetPlayers(t LiveTournament) ([]Event, error) {
	var events []Event

	provider, err := tds.providerRegistry.GetProvider(t.ProviderID)
	if err != nil {
		return events, err
	}

	url, err := provider.PlayersURL(t)
	if err != nil {
		return events, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return events, err
	}

	req.Header.Set("User-Agent", provider.UserAgent())

	resp, err := tds.http.Do(req)
	if err != nil {
		return events, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return events, err
	}

	if resp.StatusCode != http.StatusOK {
		return events, fmt.Errorf("HTTP/%s\n%s", resp.Status, body)
	}

	eventMap, err := provider.DeserializePlayers(body)
	if err != nil {
		return events, err
	}

	// Sort events in alphabetical order
	eventKeys := make([]string, len(eventMap))
	i := 0

	for k := range eventMap {
		eventKeys[i] = k
		i++
	}

	sort.Strings(eventKeys)

	for _, k := range 
	eventKeys {
		events = append(events, eventMap[k])
	}

	return events, nil
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
	DeserializePlayers(data []byte) (PlayerMap, error)
}

type USOpenProvider struct {
	DoublesEvents EventSet
}

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

func (u USOpenProvider) DeserializePlayers(data []byte) (PlayerMap, error) {
	type USOpenEvent struct {
		ID string `json:"event_id"`
		Name string `json:"event_name"`
		Seed int
	}

	type USOpenPlayer struct {
		ID string
		FirstName string `json:"first_name"`
		LastName string `json:"last_name"`
		Country string `json:"country_long"`
		Events []USOpenEvent `json:"events_entered"`
		SinglesRanking string `json:"singles_rank"`
		DoublesRanking string `json:"doubles_rank"`
	}

	type USOpenPlayerList struct {
		Players []USOpenPlayer
	}

	players := make(PlayerMap)
	var USOpenPlayers USOpenPlayerList

	if err := json.Unmarshal(data, &USOpenPlayers); err != nil {
		return players, fmt.Errorf("unmarshaling JSON response: %w", err)
	}

	for _, p := range USOpenPlayers.Players {
		for _, e := range p.Events {
			evt, ok := players[e.ID]
			if !ok {
				evt = Event{
					ID: e.ID,
					Name: e.Name,
					IsDoubles: u.DoublesEvents.Contains(e.ID),
				}
			}

			seeded := e.Seed > 0
			hasSinglesRanking := true
			singlesRanking, err := strconv.Atoi(p.SinglesRanking)
			if err != nil || p.SinglesRanking == "0" {
				singlesRanking = 0
				hasSinglesRanking = false
			}
			hasDoublesRanking := true
			doublesRanking, err := strconv.Atoi(p.DoublesRanking)
			if err != nil || p.DoublesRanking == "0" {
				doublesRanking = 0
				hasDoublesRanking = false
			}

			player := Player{
				ID: p.ID,
				Name: fmt.Sprintf("%s %s", p.FirstName, p.LastName),
				Country: p.Country,
				Seeded: seeded,
				Seed: e.Seed,
				HasSinglesRanking: hasSinglesRanking,
				SinglesRanking: singlesRanking,
				HasDoublesRanking: hasDoublesRanking,
				DoublesRanking: doublesRanking,
			}
			if seeded {
				evt.SeededPlayers = append(evt.SeededPlayers, player)
			} else {
				evt.UnseededPlayers = append(evt.UnseededPlayers, player)
			}
			players[e.ID] = evt
		}
	}

	for _, e := range players {
		sort.SliceStable(e.SeededPlayers, func(i, j int) bool {
			return e.SeededPlayers[i].Seed < e.SeededPlayers[j].Seed
		})
		sort.SliceStable(e.UnseededPlayers, func(i, j int) bool {
			if e.IsDoubles {
				return e.UnseededPlayers[i].DoublesRanking < e.UnseededPlayers[j].DoublesRanking
			} else {
				return e.UnseededPlayers[i].SinglesRanking < e.UnseededPlayers[j].SinglesRanking
			}
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

type PlayerMap map[string]Event

type Event struct {
	ID string
	Name string
	SeededPlayers []Player
	UnseededPlayers []Player
	IsDoubles bool
}

type Player struct {
	ID string
	Name string
	Country string
	Seeded bool
	Seed int
	HasSinglesRanking bool
	SinglesRanking int
	HasDoublesRanking bool
	DoublesRanking int
}

type EventSet map[string]struct{}

func NewEventSet(keys []string) EventSet {
	var empty struct{}
	es := make(EventSet)
	for _, k := range keys {
		es[k] = empty
	}
	return es
}

func (es EventSet) Contains(key string) bool {
	_, ok := es[key]
	return ok
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	oneOffTournamentProviders := []DataProvider{
		USOpenProvider{
			DoublesEvents: NewEventSet([]string{"MD", "WD", "XD", "BD", "GD", "CD", "DD", "UD", "ED"}),
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

	app := &App{
		DataService: &dataService,
		StaticFiles: staticFS,
	}

	r := chi.NewRouter()
	r.Get("/", app.currentTournaments)
	r.Get("/tournament/{id}/{year}/players", app.tournamentPlayers)

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
