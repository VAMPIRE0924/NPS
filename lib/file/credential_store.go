package file

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	credentialKeyFile = "credential.key"
	credentialPrefix  = "npsenc:v1:"
)

var credentialFieldNames = map[string]struct{}{
	"P":           {},
	"Password":    {},
	"VerifyKey":   {},
	"WebPassword": {},
}

type credentialStore struct {
	aead cipher.AEAD
}

func newCredentialStore(runPath string) (*credentialStore, error) {
	confDir := filepath.Join(runPath, "conf")
	if err := os.MkdirAll(confDir, 0750); err != nil {
		return nil, fmt.Errorf("create conf directory: %w", err)
	}
	keyPath := filepath.Join(confDir, credentialKeyFile)
	key, err := readCredentialKey(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		encrypted, scanErr := confContainsEncryptedCredentials(confDir)
		if scanErr != nil {
			return nil, scanErr
		}
		if encrypted {
			return nil, fmt.Errorf("%s is missing but encrypted data exists; restore credential.key from the same conf backup", keyPath)
		}
		key, err = createCredentialKey(keyPath)
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize credential AEAD: %w", err)
	}
	return &credentialStore{aead: aead}, nil
}

func readCredentialKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("invalid credential key %s: expected a base64-encoded 32-byte key", path)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, fmt.Errorf("secure credential key permissions: %w", err)
	}
	return key, nil
}

func createCredentialKey(path string) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate credential key: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".credential.key-*")
	if err != nil {
		return nil, fmt.Errorf("create credential key: %w", err)
	}
	tmpPath := f.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err = f.WriteString(base64.RawStdEncoding.EncodeToString(key) + "\n"); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("write credential key: %w", err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return nil, fmt.Errorf("install credential key: %w", err)
	}
	removeTmp = false
	return key, nil
}

func confContainsEncryptedCredentials(confDir string) (bool, error) {
	for _, name := range []string{"nps.conf", "clients.json", "tasks.json", "hosts.json"} {
		b, err := os.ReadFile(filepath.Join(confDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("inspect %s: %w", name, err)
		}
		if bytes.Contains(b, []byte(credentialPrefix)) {
			return true, nil
		}
	}
	return false, nil
}

// ProtectAppConfig encrypts configuration secrets on disk and returns their
// plaintext values for the already-loaded in-memory Beego configuration.
func ProtectAppConfig(runPath, configPath string) (map[string]string, error) {
	store, err := newCredentialStore(runPath)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	secretKeys := map[string]struct{}{
		"auth_key":     {},
		"public_vkey":  {},
		"web_password": {},
	}
	values := make(map[string]string)
	lines := strings.Split(string(b), "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		separator := strings.IndexByte(line, '=')
		if separator < 0 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		if _, ok := secretKeys[key]; !ok {
			continue
		}
		value := strings.TrimSpace(line[separator+1:])
		if value == "" {
			values[key] = ""
			continue
		}
		field := "nps.conf:" + key
		if strings.HasPrefix(value, credentialPrefix) {
			plaintext, err := store.decrypt(field, value)
			if err != nil {
				return nil, fmt.Errorf("decrypt %s: %w", key, err)
			}
			values[key] = plaintext
			continue
		}
		values[key] = value
		encrypted, err := store.encrypt(field, value)
		if err != nil {
			return nil, fmt.Errorf("encrypt %s: %w", key, err)
		}
		lines[i] = line[:separator+1] + encrypted
		changed = true
	}
	if changed {
		if err := atomicWriteOwnerOnly(configPath, []byte(strings.Join(lines, "\n"))); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func atomicWriteOwnerOnly(path string, data []byte) error {
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *credentialStore) encrypt(field, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plaintext), []byte(field))
	return credentialPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *credentialStore) decrypt(field, ciphertext string) (string, error) {
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, credentialPrefix))
	if err != nil || len(sealed) < s.aead.NonceSize()+s.aead.Overhead() {
		return "", errors.New("invalid encrypted credential")
	}
	nonce := sealed[:s.aead.NonceSize()]
	plaintext, err := s.aead.Open(nil, nonce, sealed[s.aead.NonceSize():], []byte(field))
	if err != nil {
		return "", errors.New("credential authentication failed; credential.key does not match this conf directory")
	}
	return string(plaintext), nil
}

func (s *credentialStore) encryptJSON(data []byte) ([]byte, error) {
	b, _, err := s.transformJSON(data, "", true)
	return b, err
}

func (s *credentialStore) decryptJSON(data []byte) ([]byte, bool, error) {
	return s.transformJSON(data, "", false)
}

func (s *credentialStore) transformJSON(data []byte, field string, encrypt bool) ([]byte, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return data, false, nil
	}
	if field == "AccountMap" || isCredentialField(field) {
		var value string
		if err := json.Unmarshal(trimmed, &value); err == nil {
			if value == "" {
				return data, false, nil
			}
			var err error
			if encrypt {
				value, err = s.encrypt(field, value)
			} else if strings.HasPrefix(value, credentialPrefix) {
				value, err = s.decrypt(field, value)
			} else {
				return data, true, nil
			}
			if err != nil {
				return nil, false, fmt.Errorf("transform %s: %w", field, err)
			}
			b, err := json.Marshal(value)
			return b, true, err
		}
	}

	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, false, err
		}
		changed := false
		for name, raw := range object {
			childField := name
			if field == "AccountMap" {
				childField = "AccountMap"
			}
			updated, itemChanged, err := s.transformJSON(raw, childField, encrypt)
			if err != nil {
				return nil, false, err
			}
			if itemChanged {
				object[name] = updated
				changed = true
			}
		}
		if !changed {
			return data, false, nil
		}
		b, err := json.Marshal(object)
		return b, true, err
	case '[':
		var array []json.RawMessage
		if err := json.Unmarshal(trimmed, &array); err != nil {
			return nil, false, err
		}
		changed := false
		for i, raw := range array {
			updated, itemChanged, err := s.transformJSON(raw, field, encrypt)
			if err != nil {
				return nil, false, err
			}
			if itemChanged {
				array[i] = updated
				changed = true
			}
		}
		if !changed {
			return data, false, nil
		}
		b, err := json.Marshal(array)
		return b, true, err
	default:
		return data, false, nil
	}
}

func isCredentialField(field string) bool {
	_, ok := credentialFieldNames[field]
	return ok
}
