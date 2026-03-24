package realtime

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
)

// StartPGListener connects to Postgres and listens for NOTIFY events on the 'realtime_events' channel.
func (h *Hub) StartPGListener() {
	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
	)

	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Realtime Listener failed to connect to Postgres: %v", err)
	}
	defer conn.Close(context.Background())

	// Subscribe to the channel
	_, err = conn.Exec(context.Background(), "LISTEN realtime_events")
	if err != nil {
		log.Fatalf("Failed to LISTEN on channel: %v", err)
	}

	log.Println("PostgreSQL Realtime Listener active on channel 'realtime_events'")

	for {
		// Wait for a notification
		notification, err := conn.WaitForNotification(context.Background())
		if err != nil {
			log.Printf("Error receiving Postgres notification: %v", err)
			continue
		}

		// Broadcast the raw JSON payload from the database to all WebSocket clients
		log.Printf("DB Event Received: %s", notification.Payload)
		h.broadcast <- []byte(notification.Payload)
	}
}
