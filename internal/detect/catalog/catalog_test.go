package catalog

import "testing"

func TestMatch(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".npmrc", "id_rsa", "config/secrets.yml", "keys/server.pem"} {
		if !c.Match(path) {
			t.Errorf("%s should match", path)
		}
	}
	for _, path := range []string{"main.go", "README.md", "src/config.go"} {
		if c.Match(path) {
			t.Errorf("%s should not match", path)
		}
	}
}

func TestMatchOrdinaryFilesDoNotMatch(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"app.log", "migration.sql", "data.json", "README.md", "main.go"} {
		if c.Match(path) {
			t.Errorf("%s should not match", path)
		}
	}
}

func TestMatchIsCaseInsensitive(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"SERVER.PEM", "KEYS/ID_RSA", ".NPMRC"} {
		if !c.Match(path) {
			t.Errorf("%s should match", path)
		}
	}
}

func TestMatchSensitivePaths(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".aws/credentials", ".docker/config.json", "secrets.yml"} {
		if !c.Match(path) {
			t.Errorf("%s should match", path)
		}
	}
}

func TestLoadReturnsSamePointer(t *testing.T) {
	first, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("Load should return the same pointer on repeated calls")
	}
}

func TestMatchRejectsBroadLookalikes(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"secrets.go", "credentials_test.go", "my_credentials.go", "id_rsa.pub", "config/secrets.txt"} {
		if c.Match(path) {
			t.Errorf("%s should not match", path)
		}
	}
}

func TestMatchExplicitCredentialPaths(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".kube/config", ".cargo/credentials", "terraform.tfstate", "service-account-prod.json"} {
		if !c.Match(path) {
			t.Errorf("%s should match", path)
		}
	}
}

func TestMatchRestoredCredentialEntries(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"terraform.tfvars",
		"vault.kdbx",
		"my.ppk",
		"service-account-prod.json",
		"sftp-config.json",
		"knife.rb",
		"secret_token.rb",
		"proftpdpasswd",
		"otr.private_key",
		".dockercfg",
		"nested/store.kdbx",
		"keys/site.ppk",
		"certs/bundle.pkcs12",
	} {
		if !c.Match(path) {
			t.Errorf("%s should match", path)
		}
	}
	for _, path := range []string{"secrets.go", "credentials_test.go", "my_credentials.go", "id_rsa.pub"} {
		if c.Match(path) {
			t.Errorf("%s should not match", path)
		}
	}
}
