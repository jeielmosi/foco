package files

import "testing"

func TestSensitiveName(t *testing.T) {
	for _, name := range []string{".env", ".env.local", "server.pem", "tls.key", "credentials.json", "id_rsa"} {
		if !IsSensitiveName(name) {
			t.Errorf("%s should be sensitive", name)
		}
	}
	for _, name := range []string{"environment.go", "keyboard.ts", "credentials_test.go"} {
		if IsSensitiveName(name) {
			t.Errorf("%s should not be sensitive", name)
		}
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.MaxFileBytes != 16*1024*1024 {
		t.Fatalf("max bytes: %d", p.MaxFileBytes)
	}
}
