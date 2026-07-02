package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/router"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("[WARN] No .env file found, reading from environment")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[ERROR] DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("[ERROR] Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("[ERROR] Database ping failed: %v", err)
	}
	log.Println("[INFO] Connected to database")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := router.SetupRouter(pool)

	log.Printf("[INFO] Server starting on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("[ERROR] Server failed to start: %v", err)
	}
}
