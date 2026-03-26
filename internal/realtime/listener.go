package realtime

import (
	"context"
	"log"

	"IOI-real-estate-backend/internal/config"

	"github.com/jackc/pgx/v5"
)

// StartPGListener connects to Postgres and listens for NOTIFY events on the 'realtime_events' channel.
func (h *Hub) StartPGListener() {
	dsn := config.Get().GetPostgresDSN()

	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Realtime Listener failed to connect to Postgres: %v", err)
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(context.Background(), "LISTEN realtime_events")
	if err != nil {
		log.Fatalf("Failed to LISTEN on channel: %v", err)
	}

	log.Println("PostgreSQL Realtime Listener active on channel 'realtime_events'")

	for {
		notification, err := conn.WaitForNotification(context.Background())
		if err != nil {
			log.Printf("Error receiving Postgres notification: %v", err)
			continue
		}

		log.Printf("DB Event Received on channel: %s", notification.Channel)
		h.broadcast <- []byte(notification.Payload)
	}
}
