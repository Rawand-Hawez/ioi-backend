package config

import (
	"log"

	"github.com/joho/godotenv"
)

// Load initializes the .env variables.
// It fails gracefully if it doesn't exist so Docker environment injection works.
func Load() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found; using environment variables from system")
	}
}
