package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

type User struct {
	Username string    `json:"username"`
	Password string    `json:"password,omitempty"`
	Admin    bool      `json:"admin"`
	Created  time.Time `json:"created"`
}

type Session struct {
	Username string    `json:"username"`
	Expires  time.Time `json:"expires"`
}

type Invite struct {
	Name    string    `json:"name"`
	Expires time.Time `json:"expires"`
}

type Node struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CredentialHash string    `json:"credential_hash,omitempty"`
	Created        time.Time `json:"created"`
	LastSeen       time.Time `json:"last_seen"`
}

type Audit struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
}

type Proxy struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	Name       string    `json:"name"`
	TargetPort int       `json:"target_port"`
	Port       int       `json:"port"`
	Scheme     string    `json:"scheme"`
	Paused     bool      `json:"paused"`
	Created    time.Time `json:"created"`
}

type State struct {
	Project       string             `json:"project"`
	Proxies       map[string]Proxy   `json:"proxies,omitempty"`
	Version       int                `json:"version"`
	Users         map[string]User    `json:"users"`
	Sessions      map[string]Session `json:"sessions"`
	Invites       map[string]Invite  `json:"invites"`
	Nodes         map[string]Node    `json:"nodes"`
	Audit         []Audit            `json:"audit"`
	AllowRegister bool               `json:"allow_register"`
}

func (s *State) Record(actor, action, target string) {
	s.Audit = append(s.Audit, Audit{time.Now().UTC(), actor, action, target})
	if len(s.Audit) > 1000 {
		s.Audit = s.Audit[len(s.Audit)-1000:]
	}
}

type Store struct{ db *bolt.DB }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filepath.Join(dir, "hub.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db}
	if err = s.Update(func(*State) error { return nil }); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func read(tx *bolt.Tx) (State, error) {
	s := State{Project: "portbridge", Proxies: map[string]Proxy{}, Version: 1, Users: map[string]User{}, Sessions: map[string]Session{}, Invites: map[string]Invite{}, Nodes: map[string]Node{}, Audit: []Audit{}}
	b := tx.Bucket([]byte("state"))
	if b != nil && b.Get([]byte("v1")) != nil {
		var marker struct {
			Project string `json:"project"`
		}
		if err := json.Unmarshal(b.Get([]byte("v1")), &marker); err != nil {
			return s, err
		}
		if marker.Project != "portbridge" {
			return s, fmt.Errorf("not a PortBridge database; use a separate data directory")
		}
		if err := json.Unmarshal(b.Get([]byte("v1")), &s); err != nil {
			return s, err
		}
	}
	if s.Proxies == nil {
		s.Proxies = map[string]Proxy{}
	}
	if s.Version != 1 {
		return s, fmt.Errorf("unsupported database schema version %d", s.Version)
	}
	return s, nil
}

func (s *Store) View(fn func(State) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		v, err := read(tx)
		if err != nil {
			return err
		}
		return fn(v)
	})
}

func (s *Store) Update(fn func(*State) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		v, err := read(tx)
		if err != nil {
			return err
		}
		if err = fn(&v); err != nil {
			return err
		}
		b, err := tx.CreateBucketIfNotExists([]byte("state"))
		if err != nil {
			return err
		}
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return b.Put([]byte("v1"), data)
	})
}

func (s *Store) Close() error { return s.db.Close() }
