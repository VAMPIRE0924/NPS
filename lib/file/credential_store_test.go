package file

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialPersistenceEncryptsAtRestAndConfBackupMigrates(t *testing.T) {
	db := newTestDb(t)
	client := newTestClient("encrypted-client")
	client.VerifyKey = "client-verify-key"
	client.WebUserName = "client-user"
	client.WebPassword = "client-web-password"
	client.Cnf.U = "proxy-user"
	client.Cnf.P = "proxy-password"
	if err := db.NewClient(client); err != nil {
		t.Fatal(err)
	}
	if client.WebUserName != "" || client.WebPassword != "" {
		t.Fatal("legacy per-client Web credentials were not discarded")
	}
	task := newTestTask("secret", client)
	task.Password = "visitor-password"
	task.MultiAccount = &MultiAccount{AccountMap: map[string]string{"alice": "multi-account-password"}}
	if err := db.NewTask(task); err != nil {
		t.Fatal(err)
	}
	db.JsonDb.StoreClientsToJsonFile()
	db.JsonDb.StoreTasksToJsonFile()

	for _, path := range []string{db.JsonDb.ClientFilePath, db.JsonDb.TaskFilePath} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range [][]byte{
			[]byte("client-verify-key"), []byte("client-web-password"),
			[]byte("proxy-password"), []byte("visitor-password"),
			[]byte("multi-account-password"),
		} {
			if bytes.Contains(b, secret) {
				t.Fatalf("%s contains plaintext credential %q", path, secret)
			}
		}
		if !bytes.Contains(b, []byte(credentialPrefix)) {
			t.Fatalf("%s does not contain encrypted credential marker", path)
		}
	}
	if client.VerifyKey != "client-verify-key" || client.Cnf.P != "proxy-password" {
		t.Fatal("in-memory values changed after encrypted persistence")
	}

	destination := t.TempDir()
	copyConfDirectory(t, filepath.Join(db.JsonDb.RunPath, "conf"), filepath.Join(destination, "conf"))
	restored := &DbUtils{JsonDb: NewJsonDb(destination)}
	restored.JsonDb.LoadClientFromJsonFile()
	restored.JsonDb.LoadTaskFromJsonFile()
	restoredClient, err := restored.GetClient(client.Id)
	if err != nil {
		t.Fatal(err)
	}
	if restoredClient.VerifyKey != client.VerifyKey || restoredClient.Cnf.P != client.Cnf.P || restoredClient.WebUserName != "" || restoredClient.WebPassword != "" {
		t.Fatal("restored client credentials do not match")
	}
	restoredTask, err := restored.ResolveTask("secret", task.Id)
	if err != nil {
		t.Fatal(err)
	}
	if restoredTask.Password != task.Password || restoredTask.MultiAccount.AccountMap["alice"] != "multi-account-password" {
		t.Fatal("restored task credentials do not match")
	}
}

func TestEncryptedConfWithoutCredentialKeyFailsClosed(t *testing.T) {
	db := newTestDb(t)
	client := newTestClient("missing-key")
	client.VerifyKey = "secret"
	if err := db.NewClient(client); err != nil {
		t.Fatal(err)
	}
	db.JsonDb.StoreClientsToJsonFile()
	if err := os.Remove(filepath.Join(db.JsonDb.RunPath, "conf", credentialKeyFile)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected encrypted conf without credential.key to fail")
		}
	}()
	_ = NewJsonDb(db.JsonDb.RunPath)
}

func TestLegacyPlaintextCredentialsMigrateOnLoad(t *testing.T) {
	runPath := t.TempDir()
	confDir := filepath.Join(runPath, "conf")
	if err := os.MkdirAll(confDir, 0750); err != nil {
		t.Fatal(err)
	}
	legacy := `{"Id":1,"VerifyKey":"legacy-vkey","WebUserName":"legacy-user","WebPassword":"legacy-password","Cnf":{"U":"u","P":"legacy-basic"},"Flow":{}}`
	if err := os.WriteFile(filepath.Join(confDir, "clients.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tasks.json", "hosts.json"} {
		if err := os.WriteFile(filepath.Join(confDir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	db := NewJsonDb(runPath)
	db.LoadClientFromJsonFile()
	client, err := db.GetClient(1)
	if err != nil {
		t.Fatal(err)
	}
	if client.VerifyKey != "legacy-vkey" || client.Cnf.P != "legacy-basic" || client.WebUserName != "" || client.WebPassword != "" {
		t.Fatal("legacy credential values changed during migration")
	}
	b, err := os.ReadFile(db.ClientFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("legacy-vkey")) || bytes.Contains(b, []byte("legacy-password")) || bytes.Contains(b, []byte("legacy-basic")) {
		t.Fatal("legacy plaintext credentials were not encrypted after load")
	}
	if bytes.Contains(b, []byte("WebUserName")) || bytes.Contains(b, []byte("WebPassword")) {
		t.Fatal("obsolete per-client Web credential fields were not removed during migration")
	}
}

func TestAppConfigSecretsEncryptAndRestoreWithConfDirectory(t *testing.T) {
	runPath := t.TempDir()
	confDir := filepath.Join(runPath, "conf")
	if err := os.MkdirAll(confDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(confDir, "nps.conf")
	config := "web_username=admin\nweb_password=admin-secret\nauth_key=api-secret\npublic_vkey=legacy-client-secret\nbridge_port=8024\n"
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := ProtectAppConfig(runPath, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if values["web_password"] != "admin-secret" || values["auth_key"] != "api-secret" || values["public_vkey"] != "legacy-client-secret" {
		t.Fatal("in-memory config secret values changed")
	}
	encrypted, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"admin-secret", "api-secret", "legacy-client-secret"} {
		if bytes.Contains(encrypted, []byte(secret)) {
			t.Fatalf("nps.conf contains plaintext secret %q", secret)
		}
	}
	if !bytes.Contains(encrypted, []byte("bridge_port=8024")) {
		t.Fatal("non-secret configuration changed")
	}

	destination := t.TempDir()
	copyConfDirectory(t, confDir, filepath.Join(destination, "conf"))
	restoredValues, err := ProtectAppConfig(destination, filepath.Join(destination, "conf", "nps.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if restoredValues["web_password"] != values["web_password"] || restoredValues["auth_key"] != values["auth_key"] || restoredValues["public_vkey"] != values["public_vkey"] {
		t.Fatal("copied conf directory did not restore configuration secrets")
	}
}

func copyConfDirectory(t *testing.T, source, destination string) {
	t.Helper()
	if err := os.MkdirAll(destination, 0750); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
