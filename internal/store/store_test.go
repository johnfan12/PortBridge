package store

import (
	"errors"
	bolt "go.etcd.io/bbolt"
	"path/filepath"
	"testing"
	"time"
)

func TestRollbackPersistenceAndExclusiveOpen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Update(func(state *State) error {
		state.Nodes["node-a"] = Node{ID: "node-a", Name: "lab", Created: time.Now()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("rollback")
	err = s.Update(func(state *State) error {
		delete(state.Nodes, "node-a")
		state.Invites["bad"] = Invite{Name: "invalid"}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal("failed update was not returned")
	}
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("two writers opened the same database")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.View(func(state State) error {
		if state.Nodes["node-a"].Name != "lab" || len(state.Invites) != 0 {
			t.Errorf("rollback/persistence failed: %+v", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestForeignDatabaseIsRejected(t *testing.T) {
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "hub.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		b, e := tx.CreateBucketIfNotExists([]byte("state"))
		if e != nil {
			return e
		}
		return b.Put([]byte("v1"), []byte(`{"version":1,"nodes":{},"proxies":{}}`))
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, err := Open(dir); err == nil {
		s.Close()
		t.Fatal("accepted NodeBridge database")
	}
}
