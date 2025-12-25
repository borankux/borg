package main

import (
	"log"
	"net/http"
	"os"

	"borg/mothership/internal/api"
	"borg/mothership/internal/models"
	"borg/mothership/internal/queue"
	"borg/mothership/internal/storage"
	"borg/mothership/internal/websocket"
	
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}
	
	// Database connection
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=borg port=5432 sslmode=disable"
	}
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	
	// Run migrations
	if err := models.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	
	// Initialize storage
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./storage"
	}
	
	storageService, err := storage.NewStorage(storagePath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	
	// Initialize queue
	q := queue.NewQueue(db)
	
	// Initialize WebSocket hub
	hub := websocket.NewHub()
	go hub.Run()
	
	// Initialize REST API server
	apiServer := api.NewServer(db, q, hub, storageService)
	
	// Start HTTP server
	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}
	
	log.Printf("Starting HTTP server on 0.0.0.0:%s", httpPort)
	log.Printf("WebSocket endpoint: ws://0.0.0.0:%s/ws", httpPort)
	log.Printf("REST API endpoint: http://0.0.0.0:%s/api/v1", httpPort)
	log.Printf("Web dashboard: http://0.0.0.0:%s", httpPort)
	log.Printf("Runner API endpoint: http://0.0.0.0:%s/api/v1/runners", httpPort)
	
	if err := http.ListenAndServe("0.0.0.0:"+httpPort, apiServer.GetRouter()); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

