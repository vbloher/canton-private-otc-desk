package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	maintenanceDatabase = "postgres"
	duplicateDatabase   = "42P04"
)

type Config struct {
	User     string `env:"POSTGRES_USER,required"`
	Password string `env:"POSTGRES_PASSWORD,required"`
	Port     string `env:"POSTGRES_PORT,required"`
	Host     string `env:"POSTGRES_HOST,required"`
	DBName   string `env:"POSTGRES_DB_NAME,required"`
}

func (c Config) URL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     net.JoinHostPort(c.Host, c.Port),
		Path:     c.DBName,
		RawQuery: "sslmode=disable",
	}

	return u.String()
}

type Pool struct {
	*pgxpool.Pool
}

func EnsureDatabase(ctx context.Context, c Config) (err error) {
	admin := c
	admin.DBName = maintenanceDatabase

	conn, err := pgx.Connect(ctx, admin.URL())
	if err != nil {
		return fmt.Errorf("pgx.Connect: %w", err)
	}

	defer func() {
		err = errors.Join(err, conn.Close(ctx))
	}()

	var exists bool

	err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, c.DBName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("row.Scan: %w", err)
	}

	if exists {
		return nil
	}

	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{c.DBName}.Sanitize())

	var failed *pgconn.PgError
	if errors.As(err, &failed) && failed.Code == duplicateDatabase {
		return nil
	}

	if err != nil {
		return fmt.Errorf("conn.Exec: %w", err)
	}

	return nil
}

func New(ctx context.Context, c Config) (*Pool, error) {
	dsn := fmt.Sprintf("user=%s password=%s port=%s host=%s dbname=%s",
		c.User,
		c.Password,
		c.Port,
		c.Host,
		c.DBName,
	)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("pool.Ping: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

func (p *Pool) Close() {
	p.Pool.Close()

	log.Info().Msg("postgres: closed")
}
