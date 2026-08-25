package store

import (
	"context"
	"errors"
)

// PlayerID is the database primary key for a player.
type PlayerID int64

// Player holds the persistent state of a player.
type Player struct {
	ID      PlayerID
	Name    string
	Element uint8
	Level   uint32
	XP      uint32
	Gold    uint32
}

// PlayerStore abstracts all persistence operations.
// Callers never touch the underlying database — swap the implementation
// (sqlite.go → postgres.go) without changing a single line outside this package.
type PlayerStore interface {
	// Register creates a new player. Returns ErrNameTaken if the name is in use.
	Register(ctx context.Context, name, password string, element uint8) (Player, error)
	// Authenticate checks credentials. Returns ErrBadCredentials on failure.
	Authenticate(ctx context.Context, name, password string) (Player, error)
	// GetByID loads a player by primary key. Used by game servers to refresh
	// state after validating a JWT (level/xp may have changed since token was issued).
	GetByID(ctx context.Context, id PlayerID) (Player, error)
	// Save persists progression. Called when a player disconnects.
	Save(ctx context.Context, id PlayerID, level, xp, gold uint32) error
	Close() error
}

var (
	ErrNameTaken      = errors.New("name already taken")
	ErrBadCredentials = errors.New("invalid name or password")
)
