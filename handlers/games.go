package handlers

import (
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/models"
	"nfl-app-go/services"
)

// GameHandler handles game-related HTTP requests
type GameHandler struct {
	templates   *template.Template
	gameService services.GameService
}

// NewGameHandler creates a new game handler
func NewGameHandler(templates *template.Template, gameService services.GameService) *GameHandler {
	return &GameHandler{
		templates:   templates,
		gameService: gameService,
	}
}

// GetGames handles GET /games - displays all games
func (h *GameHandler) GetGames(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	games, err := h.gameService.GetGames()
	if err != nil {
		log.Printf("GameHandler: Error fetching games: %v", err)
		http.Error(w, "Unable to fetch games", http.StatusInternalServerError)
		return
	}
	
	log.Printf("GameHandler: Rendering %d games", len(games))
	
	data := struct {
		Games []models.Game
		Title string
	}{
		Games: games,
		Title: "NFL Games & Scores",
	}

	err = h.templates.ExecuteTemplate(w, "games.html", data)
	if err != nil {
		log.Printf("GameHandler: Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	log.Printf("HTTP: Successfully served %s %s", r.Method, r.URL.Path)
}