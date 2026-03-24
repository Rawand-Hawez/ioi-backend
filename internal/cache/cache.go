package cache

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	Client *redis.Client
	ctx    = context.Background()
)

// InitCache initializes the connection to the Dragonfly instance.
func InitCache() {
	addr := fmt.Sprintf("localhost:%s", os.Getenv("DRAGONFLY_PORT"))
	if addr == "localhost:" {
		addr = "localhost:6379"
	}

	Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // Dragonfly typically runs without password in local skeletons
		DB:       0,  // Standard default DB
	})

	// Perform a health check
	if err := Client.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Unable to connect to Dragonfly at %s: %v", addr, err)
	} else {
		log.Printf("Connected to Dragonfly instance at %s", addr)
	}
}

// Set stores a value in the cache with an optional TTL.
func Set(key string, value interface{}, expiration time.Duration) error {
	return Client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value from the cache.
func Get(key string) (string, error) {
	return Client.Get(ctx, key).Result()
}

// Delete removes a key from the cache.
func Delete(key string) error {
	return Client.Del(ctx, key).Err()
}
