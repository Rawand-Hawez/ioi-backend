package handlers

import (
	"log"

	"IOI-real-estate-backend/internal/config"
	"IOI-real-estate-backend/internal/db/pool"
)

func InitDB() {
	cfg := config.Get()
	dsn := cfg.GetPostgresDSN()

	if err := pool.Init(dsn); err != nil {
		log.Fatalf("Failed to initialize database connection: %v", err)
	}

	log.Println("Database connection pool initialized successfully")
}
