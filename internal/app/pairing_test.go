package app

import (
	"encoding/json"
	"net"
	"portbridge/internal/store"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPairingDoesNotAllocatePortsAndInviteIsAtomic(t *testing.T) {
	base := portPool(t, 1)
	f := newFixture(t, base, base)
	occupied, err := net.Listen("tcp", ":"+itoa(base))
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	pair, _ := DecodePair(f.invite(t, "first"))
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := f.request(t, "POST", "/api/enroll", map[string]string{"token": pair.Token}, nil)
			codes <- r.StatusCode
			r.Body.Close()
		}()
	}
	wg.Wait()
	close(codes)
	success, denied := 0, 0
	for c := range codes {
		if c == 201 {
			success++
		}
		if c == 400 {
			denied++
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("single use: %d successes, %d refusals", success, denied)
	}
	second, _ := DecodePair(f.invite(t, "second"))
	mustStatus(t, f.request(t, "POST", "/api/enroll", map[string]string{"token": second.Token}, nil), 201)
	if err := f.hub.Store.View(func(s store.State) error {
		if len(s.Nodes) != 2 || len(s.Proxies) != 0 {
			t.Fatal("pairing created a service or lost a node")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expired, _ := DecodePair(f.invite(t, "expired"))
	if err := f.hub.Store.Update(func(s *store.State) error {
		i := s.Invites[hashToken(expired.Token)]
		i.Expires = time.Now().Add(-time.Second)
		s.Invites[hashToken(expired.Token)] = i
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.request(t, "POST", "/api/enroll", map[string]string{"token": expired.Token}, nil), 400)
	// Distinct link namespace prevents consuming a NodeBridge invitation by accident.
	if _, err := DecodePair(strings.Replace(EncodePair(pair), "portbridge://", "nodebridge://", 1)); err == nil {
		t.Fatal("accepted foreign project link")
	}
	if err := f.hub.Store.View(func(s store.State) error {
		b, _ := json.Marshal(s.Nodes)
		if strings.Contains(string(b), `"port":`) {
			t.Fatal("node pairing stores a public service port")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
