package migrate

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rs/zerolog/log"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	gomigrate "github.com/golang-migrate/migrate/v4"
)

func Up(dsn string, fsys fs.FS) error {
	source, err := iofs.New(fsys, "postgres")
	if err != nil {
		return fmt.Errorf("iofs.New: %w", err)
	}

	m, err := gomigrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return fmt.Errorf("migrate.NewWithSourceInstance: %w", err)
	}

	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			log.Error().AnErr("source", sourceErr).AnErr("db", dbErr).Msg("migrate: close")
		}
	}()

	err = m.Up()
	if err != nil && !errors.Is(err, gomigrate.ErrNoChange) {
		return fmt.Errorf("m.Up: %w", err)
	}

	log.Info().Msg("migrate: schema is up to date")

	return nil
}
