package store

import (
	"context"
	"testing"
)

func openTestDB(t *testing.T) PlayerStore {
	t.Helper()
	st, err := Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestRegisterAndAuthenticate(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	p, err := st.Register(ctx, "kevin", "secret", 1)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if p.Name != "kevin" || p.Level != 1 || p.Element != 1 {
		t.Fatalf("unexpected player: %+v", p)
	}

	p2, err := st.Authenticate(ctx, "kevin", "secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if p2.ID != p.ID {
		t.Fatalf("id mismatch: %d vs %d", p2.ID, p.ID)
	}
}

func TestDuplicateName(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	st.Register(ctx, "dup", "pw", 0) //nolint:errcheck
	_, err := st.Register(ctx, "dup", "pw2", 0)
	if err != ErrNameTaken {
		t.Fatalf("expected ErrNameTaken, got %v", err)
	}
}

func TestBadCredentials(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	st.Register(ctx, "player", "right", 2) //nolint:errcheck
	_, err := st.Authenticate(ctx, "player", "wrong")
	if err != ErrBadCredentials {
		t.Fatalf("expected ErrBadCredentials, got %v", err)
	}
	_, err = st.Authenticate(ctx, "nobody", "pw")
	if err != ErrBadCredentials {
		t.Fatalf("expected ErrBadCredentials for unknown user, got %v", err)
	}
}

func TestSave(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	p, _ := st.Register(ctx, "saver", "pw", 1)
	if err := st.Save(ctx, p.ID, 15, 370, 500); err != nil {
		t.Fatalf("save: %v", err)
	}
	p2, _ := st.Authenticate(ctx, "saver", "pw")
	if p2.Level != 15 || p2.XP != 370 || p2.Gold != 500 {
		t.Fatalf("expected level=15 xp=370 gold=500, got %+v", p2)
	}
}
