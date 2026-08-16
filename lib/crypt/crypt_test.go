package crypt

import "testing"

func TestGetRandomStringUsesExpectedAlphabetAndLength(t *testing.T) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	value := GetRandomString(64)
	if len(value) != 64 {
		t.Fatalf("expected 64 characters, got %d", len(value))
	}
	for _, ch := range value {
		found := false
		for _, allowed := range alphabet {
			if ch == allowed {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("unexpected character %q", ch)
		}
	}
}
