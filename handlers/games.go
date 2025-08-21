package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"strings"
	"time"
)

// GameHandler handles game-related HTTP requests
type GameHandler struct {
	templates   *template.Template
	gameService services.GameService
	sseClients  map[chan string]bool
}

// NewGameHandler creates a new game handler
func NewGameHandler(templates *template.Template, gameService services.GameService) *GameHandler {
	return &GameHandler{
		templates:   templates,
		gameService: gameService,
		sseClients:  make(map[chan string]bool),
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

// SSEHandler handles Server-Sent Events for real-time game updates
func (h *GameHandler) SSEHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("SSE: New client connected from %s", r.RemoteAddr)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel
	clientChan := make(chan string, 10)
	h.sseClients[clientChan] = true

	// Send initial connection confirmation
	fmt.Fprintf(w, "event: connected\n")
	fmt.Fprintf(w, "data: SSE connection established\n\n")
	w.(http.Flusher).Flush()

	// Handle client disconnect
	defer func() {
		delete(h.sseClients, clientChan)
		close(clientChan)
		log.Printf("SSE: Client disconnected from %s", r.RemoteAddr)
	}()

	// Keep connection alive and send updates
	for {
		select {
		case message := <-clientChan:
			fmt.Fprintf(w, "event: gameUpdate\n")
			// Split message into lines and prefix each with "data: "
			lines := strings.Split(message, "\n")
			for _, line := range lines {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprintf(w, "\n")
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			// Send keepalive
			fmt.Fprintf(w, "event: keepalive\n")
			fmt.Fprintf(w, "data: ping\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

// BroadcastUpdate sends game updates to all connected SSE clients
func (h *GameHandler) BroadcastUpdate() {
	games, err := h.gameService.GetGames()
	if err != nil {
		log.Printf("SSE: Error fetching games for broadcast: %v", err)
		return
	}

	// Render games HTML
	data := struct {
		Games []models.Game
	}{
		Games: games,
	}

	// Use a buffer to capture template output
	var htmlBuffer strings.Builder
	err = h.templates.ExecuteTemplate(&htmlBuffer, "game-list", data)
	if err != nil {
		log.Printf("SSE: Template error for broadcast: %v", err)
		return
	}

	htmlContent := htmlBuffer.String()
	log.Printf("SSE: Generated HTML content length: %d", len(htmlContent))
	
	if len(htmlContent) == 0 {
		log.Printf("SSE: Warning - HTML content is empty!")
		return
	}
	
	// Send to all connected clients
	for clientChan := range h.sseClients {
		select {
		case clientChan <- htmlContent:
		default:
			// Client channel is full, skip
		}
	}
	
	log.Printf("SSE: Broadcasted update to %d clients", len(h.sseClients))
}

