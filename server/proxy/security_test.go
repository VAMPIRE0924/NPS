package proxy

import "testing"

func TestValidateWebCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{name: "strong", username: "admin", password: "correct-horse-battery-staple", wantErr: false},
		{name: "empty username", username: "", password: "correct-horse-battery-staple", wantErr: true},
		{name: "short password", username: "admin", password: "12345678901", wantErr: true},
		{name: "placeholder", username: "admin", password: "CHANGE_ME_BEFORE_START", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotErr := validateWebCredentials(tt.username, tt.password) != nil; gotErr != tt.wantErr {
				t.Fatalf("validateWebCredentials() error = %v, wantErr %v", gotErr, tt.wantErr)
			}
		})
	}
}
