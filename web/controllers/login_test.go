package controllers

import (
	"sync"
	"testing"
	"time"
)

func TestLoginAttemptStoreBlocksAndExpires(t *testing.T) {
	store := &loginAttemptStore{records: make(map[string]record)}
	now := time.Unix(1800000000, 0)
	for i := 0; i < 10; i++ {
		store.failure("127.0.0.1", now)
	}
	if !store.blocked("127.0.0.1", now) {
		t.Fatal("expected ten failures to block login")
	}
	if store.blocked("127.0.0.1", now.Add(time.Minute)) {
		t.Fatal("expected login block to expire after one minute")
	}
}

func TestLoginAttemptStoreConcurrentFailures(t *testing.T) {
	store := &loginAttemptStore{records: make(map[string]record)}
	now := time.Unix(1800000000, 0)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.failure("127.0.0.1", now)
		}()
	}
	wg.Wait()
	if !store.blocked("127.0.0.1", now) {
		t.Fatal("expected concurrent failures to be counted")
	}
	store.success("127.0.0.1")
	if store.blocked("127.0.0.1", now) {
		t.Fatal("successful login must clear failures")
	}
}
