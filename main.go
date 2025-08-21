package main

import (
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/handlers"
	"nfl-app-go/services"

	"github.com/gorilla/mux"
)

func main() {
	// Parse templates
	templates, err := template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Error parsing templates:", err)
	}

	// Create services
	gameService := services.NewDemoGameService()
	
	// Create handlers
	gameHandler := handlers.NewGameHandler(templates, gameService)

	// Setup routes
	r := mux.NewRouter()
	
	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	
	// Game routes
	r.HandleFunc("/", gameHandler.GetGames).Methods("GET")
	r.HandleFunc("/games", gameHandler.GetGames).Methods("GET")

	// Start server
	log.Println("Server starting on 127.0.0.1:8080")
	log.Println("Visit: http://localhost:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", r))
}