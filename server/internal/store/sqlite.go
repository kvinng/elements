package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

type sqlStore struct{ db *sql.DB }

// Open opens (or creates) a SQL store.
// Switch databases by changing driverName + dataSourceName:
//
//	SQLite:     Open("sqlite", "file:./elements.db")
//	PostgreSQL: Open("postgres", "postgres://user:pass@host/db")
//
// The returned PlayerStore is the only interface callers need.
func Open(driverName, dataSourceName string) (PlayerStore, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &sqlStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS players (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    UNIQUE NOT NULL,
    password   TEXT    NOT NULL,
    element    INTEGER NOT NULL DEFAULT 0,
    level      INTEGER NOT NULL DEFAULT 1,
    xp         INTEGER NOT NULL DEFAULT 0,
    gold       INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT 0
);`

func (s *sqlStore) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Idempotent: add gold column to tables created before this migration.
	_, err := s.db.Exec(`ALTER TABLE players ADD COLUMN gold INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}

func (s *sqlStore) Register(ctx context.Context, name, password string, element uint8) (Player, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Player{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO players(name, password, element, level, xp, updated_at) VALUES (?,?,?,1,0,?)`,
		name, string(hash), element, time.Now().Unix(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Player{}, ErrNameTaken
		}
		return Player{}, err
	}
	id, _ := res.LastInsertId()
	return Player{ID: PlayerID(id), Name: name, Element: element, Level: 1, XP: 0}, nil
}

func (s *sqlStore) Authenticate(ctx context.Context, name, password string) (Player, error) {
	var p Player
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, password, element, level, xp, gold FROM players WHERE name = ?`, name).
		Scan(&p.ID, &p.Name, &hash, &p.Element, &p.Level, &p.XP, &p.Gold)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrBadCredentials
	}
	if err != nil {
		return Player{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return Player{}, ErrBadCredentials
	}
	return p, nil
}

func (s *sqlStore) GetByID(ctx context.Context, id PlayerID) (Player, error) {
	var p Player
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, element, level, xp, gold FROM players WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Element, &p.Level, &p.XP, &p.Gold)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrBadCredentials
	}
	return p, err
}

func (s *sqlStore) Save(ctx context.Context, id PlayerID, level, xp, gold uint32) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE players SET level=?, xp=?, gold=?, updated_at=? WHERE id=?`,
		level, xp, gold, time.Now().Unix(), id,
	)
	return err
}

func (s *sqlStore) Close() error { return s.db.Close() }

// isUniqueViolation detects UNIQUE constraint errors from modernc/sqlite.
// When migrating to PostgreSQL, the postgres driver returns a *pq.Error with
// Code "23505" — write a pgStore that checks that instead.
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
