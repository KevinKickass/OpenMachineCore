package storage

import (
	"context"
	"fmt"

	"github.com/KevinKickass/OpenMachineCore/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresClient struct {
	pool *pgxpool.Pool
}

func NewPostgresClient(cfg config.DatabaseConfig) (*PostgresClient, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse pool config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxConnections)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Connection testen
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresClient{pool: pool}, nil
}

func (p *PostgresClient) Close() {
	p.pool.Close()
}

func (p *PostgresClient) Pool() *pgxpool.Pool {
	return p.pool
}
