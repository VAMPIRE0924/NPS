package controllers

import (
	"sync"
	"testing"
	"time"

	"ehang.io/nps/lib/file"
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

func TestClientWebLoginUsesOnlyUserAndVerifyKey(t *testing.T) {
	client := &file.Client{
		Status:      true,
		VerifyKey:   "device-verify-key",
		WebUserName: "legacy-name",
		WebPassword: "legacy-password",
	}
	if !matchesClientWebLogin(client, "user", client.VerifyKey) {
		t.Fatal("fixed user + Client VerifyKey login must remain available")
	}
	if matchesClientWebLogin(client, client.WebUserName, client.WebPassword) {
		t.Fatal("legacy per-client Web username/password must not authenticate")
	}
	if matchesClientWebLogin(client, "user", "wrong-key") {
		t.Fatal("wrong VerifyKey authenticated")
	}
}

func TestDisabledOrHiddenClientCannotLogIn(t *testing.T) {
	for _, client := range []*file.Client{
		{Status: false, VerifyKey: "key"},
		{Status: true, NoDisplay: true, VerifyKey: "key"},
	} {
		if matchesClientWebLogin(client, "user", "key") {
			t.Fatal("disabled or hidden Client authenticated")
		}
	}
}
