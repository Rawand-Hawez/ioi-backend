package cache

import (
	"context"
	"fmt"
	"log"
	"time"

	"ioibackend/internal/config"

	"github.com/redis/go-redis/v9"
)

var (
	Client  *redis.Client
	healthy bool
)

// InitCache initializes the connection to the Dragonfly instance.
func InitCache() {
	cfg := config.Get()
	addr := fmt.Sprintf("%s:%s", cfg.Dragonfly.Host, cfg.Dragonfly.Port)

	Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Dragonfly.Password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Dragonfly unavailable at %s: %v (cache operations will fail)", addr, err)
		healthy = false
	} else {
		log.Printf("Connected to Dragonfly at %s", addr)
		healthy = true
	}
}

// IsHealthy reports whether the cache connected successfully at init.
func IsHealthy() bool { return healthy }

// Set stores a value in the cache with an optional TTL.
func Set(key string, value interface{}, expiration time.Duration) error {
	return Client.Set(context.Background(), key, value, expiration).Err()
}

// Get retrieves a value from the cache.
func Get(key string) (string, error) {
	return Client.Get(context.Background(), key).Result()
}

// Delete removes a key from the cache.
func Delete(key string) error {
	return Client.Del(context.Background(), key).Err()
}
