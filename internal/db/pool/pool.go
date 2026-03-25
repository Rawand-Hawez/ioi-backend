package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// contextKey is the type for context keys in this package
type contextKey string

const (
	// TxKey is the context key for storing the current transaction
	TxKey contextKey = "tx"
	// ConnKey is the context key for storing the current connection
	ConnKey contextKey = "conn"
)

// Pool wraps pgxpool.Pool with GUC injection support
type Pool struct {
	*pgxpool.Pool
}

var (
	instance *Pool
	once     sync.Once
)

// Init initializes the global connection pool with production-ready limits
func Init(dsn string) error {
	var initErr error
	once.Do(func() {
		poolConfig, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			initErr = fmt.Errorf("failed to parse DSN: %w", err)
			return
		}
		poolConfig.MaxConns = 25
		poolConfig.MinConns = 5
		poolConfig.MaxConnLifetime = 30 * time.Minute
		poolConfig.MaxConnIdleTime = 5 * time.Minute
		poolConfig.HealthCheckPeriod = 30 * time.Second

		pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err != nil {
			initErr = fmt.Errorf("failed to create connection pool: %w", err)
			return
		}
		instance = &Pool{Pool: pool}
	})
	return initErr
}

// Get returns the global pool instance
func Get() *Pool {
	return instance
}

// Acquire gets a connection from the pool
func (p *Pool) Acquire(ctx context.Context) (*Conn, error) {
	conn, err := p.Pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	return &Conn{Conn: conn}, nil
}

// Conn wraps pgxpool.Conn with GUC injection support
type Conn struct {
	*pgxpool.Conn
}

// BeginTx starts a new transaction with GUC variables set
func (c *Conn) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := c.Conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return tx, nil
}

// InjectGUC sets JWT claims as PostgreSQL GUC variables within a transaction
// Uses parameterized set_config() to prevent SQL injection
func InjectGUC(ctx context.Context, tx pgx.Tx, claims map[string]interface{}) error {
	if claims == nil {
		return nil
	}

	// set_config(name, value, is_local) — is_local=true scopes to current transaction
	if sub, ok := claims["sub"].(string); ok && sub != "" {
		if _, err := tx.Exec(ctx, "SELECT set_config('request.jwt.claim.sub', $1, true)", sub); err != nil {
			return fmt.Errorf("failed to set GUC variable sub: %w", err)
		}
	}

	if role, ok := claims["role"].(string); ok && role != "" {
		if _, err := tx.Exec(ctx, "SELECT set_config('request.jwt.claim.role', $1, true)", role); err != nil {
			return fmt.Errorf("failed to set GUC variable role: %w", err)
		}
	}

	if email, ok := claims["email"].(string); ok && email != "" {
		if _, err := tx.Exec(ctx, "SELECT set_config('request.jwt.claim.email', $1, true)", email); err != nil {
			return fmt.Errorf("failed to set GUC variable email: %w", err)
		}
	}

	return nil
}

// WithTx executes a function within a transaction with GUC variables injected
func (p *Pool) WithTx(ctx context.Context, claims map[string]interface{}, fn func(pgx.Tx) error) error {
	conn, err := p.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Inject GUC variables before executing the function
	if err := InjectGUC(ctx, tx, claims); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("failed to inject GUC variables: %w", err)
	}

	// Execute the function
	if err := fn(tx); err != nil {
		tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}

// Close closes the connection pool
func (p *Pool) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
}
