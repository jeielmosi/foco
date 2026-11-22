package detect

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCredentialsFindsHandWrittenSecrets(t *testing.T) {
	for _, c := range []struct{ path, text string }{
		{"a.json", `{"aws":{"accessKeyId":"AKIAIOSFODNN7EXAMPLE"}}`},
		{"a.json", `{"legacy_servers":["db-prod-01.empresa.local:S3nh@Prod#2026"]}`},
		{"a.json", `{"legacy_servers":["://parceiro.com.br:FtpUsr$998877"]}`},
		{"a.json", `{"fallback_connection":"10.240.15.88:Admin#Backup2026"}`},
		{"a.json", `{"redis-cache.interno":"R3d1s_P@ssw0rd$99"}`},
		{"a.json", `{"10.240.15.88":"Admin#Backup2026"}`},
		{"a.json", "{\n  \"database_postgres\": \"P@ssw0rd_Pr0d_2026!\"\n}"},
		{"a.json", "{\n  \"smtp_email_service\": \"Smtp#Secr3t_9981\"\n}"},
		{"a.py", `DB_PASSWORD = "hunter2!Prod"`},
		{".env", "API_TOKEN=9f2c4e7a1b3d5f60\n"},
		{"a.py", `password = "correct-horse-battery-staple!"`},
		{"a.py", `DATABASE_URL = "postgres://admin:S3cretPa55!@db.example.com/app"`},
		{"a.py", `uri = "mongodb+srv://admin_prod:S3nh@SuperSecreta2026!@cluster0.mongodb.net/prod"`},
		{"a.py", `redis = "localhost:S3cretPa55!"`},
		{"a.go", `dbPassword := "hunter2!Prod"`},
		{"a.rb", `:password => "hunter2!Prod"`},
		{"a.json", `{"aws_secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}`},
	} {
		if credentialMatch(c.path, c.text) == "" {
			t.Errorf("missed %s: %s", c.path, c.text)
		}
	}
}

// handwrittenCredentials are the 28 credentials of the four comparison texts,
// one per file: AWS, GitHub, Stripe, JWT, private keys and MongoDB URIs that
// provider rules catch, plus passwords, host:password pairs and host-keyed
// passwords that only the credential heuristics catch. Token prefixes are
// split so this source file is not itself flagged by secret scanners.
var handwrittenCredentials = []struct{ name, content string }{
	{"t1-aws-key-id", "{\"aws\":{\"accessKeyId\":\"AK" + "IAIOSFODNN7EXAMPLE\"}}"},
	{"t1-aws-secret", "{\"aws\":{\"secretAccessKey\":\"wJalrX" + "UtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"}}"},
	{"t1-github-token", "{\"githubToken\":\"gh" + "p_16C7e42F292c6912E7710c838347Ae178B4a\"}"},
	{"t1-jwt", "{\"jwtSecret\":\"eyJhb" + "GciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkFkbWluIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c\"}"},
	{"t1-mongodb-uri", "{\"databaseUri\":\"mongodb" + "+srv://admin_prod:S3nh@SuperSecreta2026!@cluster0.mongodb.net/prod\"}"},
	{"t1-private-key", "{\"privateKey\":\"-----BEGIN PR" + "IVATE KEY-----\\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQD1234567890example\\n-----END PRIVATE KEY-----\"}"},
	{"t1-stripe", "{\"stripeSecret\":\"sk_l" + "ive_51N9x8L2eZvKYlo2C9f8d7a6s5d4f3g2h1j\"}"},
	{"t2-fallback", "{\"fallback_connection\": \"10.240.15.88:Admin#Backup2026\"}"},
	{"t2-legacy-1", "{\"legacy_servers\": [\"db-prod-01.empresa.local:S3nh@Prod#2026\"]}"},
	{"t2-legacy-2", "{\"legacy_servers\": [\"redis-cache.interno:R3d1s_P@ssw0rd!\"]}"},
	{"t2-legacy-3", "{\"legacy_servers\": [\"://parceiro.com.br:FtpUsr$998877\"]}"},
	{"t2-legacy-4", "{\"legacy_servers\": [\"://empresa.com:M@ilS3rver_2026!\"]}"},
	{"t2-legacy-5", "{\"legacy_servers\": [\"ssh-bastion-02.cloud:Root@Access#1234\"]}"},
	{"t2-legacy-6", "{\"legacy_servers\": [\"rabbitmq-01.cluster:MqAdmin!987654\"]}"},
	{"t3-host-1", "{\"db-prod-01.empresa.local\": \"S3nh@Prod#2026!\"}"},
	{"t3-host-2", "{\"redis-cache.interno\": \"R3d1s_P@ssw0rd$99\"}"},
	{"t3-host-3", "{\"://parceiro.com.br\": \"FtpUsr#998877aA\"}"},
	{"t3-host-4", "{\"://empresa.com\": \"M@ilS3rver_2026!\"}"},
	{"t3-host-5", "{\"ssh-bastion-02.cloud\": \"Root@Access#1234\"}"},
	{"t3-host-6", "{\"rabbitmq-01.cluster\": \"MqAdmin!987654xZ\"}"},
	{"t3-host-7", "{\"10.240.15.88\": \"Admin#Backup2026\"}"},
	{"t4-aws-access-key", "{\n  \"aws_access_key\": \"AK" + "IAIOSFODNN7EXAMPLE\"\n}"},
	{"t4-aws-secret-key", "{\n  \"aws_secret_key\": \"wJalrX" + "UtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n}"},
	{"t4-database-postgres", "{\n  \"database_postgres\": \"P@ssw0rd_Pr0d_2026!\"\n}"},
	{"t4-jwt-signing-secret", "{\n  \"jwt_signing_secret\": \"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\"\n}"},
	{"t4-redis-cache-token", "{\n  \"redis_cache_token\": \"rd_tok_88a7b6c5d4e3f2a1\"\n}"},
	{"t4-smtp-email-service", "{\n  \"smtp_email_service\": \"Smtp#Secr3t_9981\"\n}"},
	{"t4-stripe-api-secret", "{\n  \"stripe_api_secret\": \"sk_l" + "ive_51N9x8K2eZvKYlo2C9x8K2eZv\"\n}"},
}

func TestUnionExcludesEveryHandwrittenCredential(t *testing.T) {
	titus, err := NewTitus()
	if err != nil {
		t.Fatal(err)
	}
	better, err := NewBetterleaks(1)
	if err != nil {
		t.Fatal(err)
	}
	union := NewUnion(titus, better, loadCatalog(t))
	defer union.Close()
	dir := t.TempDir()
	for _, sample := range handwrittenCredentials {
		path := filepath.Join(dir, sample.name+".json")
		if err := os.WriteFile(path, []byte(sample.content+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		excluded, err := union.DetectPath(context.Background(), path)
		if err != nil || !excluded {
			t.Errorf("%s: excluded=%v err=%v", sample.name, excluded, err)
		}
	}
}

func TestCredentialsIgnoresOrdinaryCode(t *testing.T) {
	for _, c := range []struct{ path, text string }{
		{"a.json", `{"slackWebhook":"https://slack.com/"}`},
		{"a.go", `token := parser.Next()`},
		{"a.rb", `database: "storage/development.sqlite3"`},
		{"a.js", `const server = "localhost:8080";`},
		{"a.json", `{"example.com": "Example Domain"}`},
		{"a.py", `password = os.environ["DB_PASSWORD"]`},
		{"a.yml", "password: ${DB_PASSWORD}\n"},
		{"a.rb", `version = "5.5.5-10.6.5-MariaDB-1:10.6.5+maria~focal"`},
		{"a.py", `relay = "127.0.0.1:{relay_ports[0]}"`},
		{"a.md", `Set ADMIN_PASSWORD = "<your password>" before running.`},
		{"a.py", `password_change_template = "custom_admin/password_change_form.html"`},
		{"a.py", `data = {"password1": "FORBIDDEN_VALUE2"}`},
		{"tox.ini", "postgres: -rtests/requirements/postgres.txt\n"},
		{"a.json", `{"Unexpected_token_1012": "予期しないトークンです。"}`},
		{"a.py", `FineTuningAPIKey.REPORTS: "reports:disabled-organizations"`},
		{"a.py", `{"store.background-grouping-config-id": "mobile:2021-02-12"}`},
		{"a.py", `{"bootstrap.servers": "my.server:9092"}`},
		{"a.py", `API_KEY_HEADER = "X-Api-Key-V2"`},
		{"a.js", `const bypassList = "Foo-Bar1-Baz"`},
		{"a.yml", "passed: 2024-01-01T00:00Z\n"},
		{"a.py", `secret = "test-secret-123A"`},
		{"a.ts", `const tokenizer = "Tok3n!zer_v2"`},
		{"a.py", `url = "https://user@example.com/path"`},
		{"a.py", `url = "postgres://app:${DB_PASSWORD}@db/app"`},
		{"a.py", `password = "Please-enter"`},
		{"a.rb", `password: "queenbee#passwords"`},
		{"a.py", `{"PASSWORD": "some:password"}`},
		{"a.rb", `password: "[FILTERED]"`},
	} {
		if match := credentialMatch(c.path, c.text); match != "" {
			t.Errorf("false positive %q in %s", match, c.text)
		}
	}
}

// FOCO_CREDENTIALS_CORPUS=dir1:dir2 lists every file the engine flags with
// the matching text, for reviewing false positives on real repositories.
func TestCredentialsCorpusReport(t *testing.T) {
	roots := os.Getenv("FOCO_CREDENTIALS_CORPUS")
	if roots == "" {
		t.Skip("set FOCO_CREDENTIALS_CORPUS to run")
	}
	files, hits := 0, 0
	for _, root := range filepath.SplitList(roots) {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || len(data) > 1<<20 || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
				return nil
			}
			files++
			if match := credentialMatch(path, string(data)); match != "" {
				hits++
				t.Logf("%s\t%s", path, strings.ReplaceAll(match, "\n", " "))
			}
			return nil
		})
	}
	t.Logf("files=%d flagged=%d", files, hits)
}

func BenchmarkCredentialRules(b *testing.B) {
	path := os.Getenv("FOCO_CREDENTIALS_BENCH_FILE")
	if path == "" {
		b.Skip("set FOCO_CREDENTIALS_BENCH_FILE")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	text := string(data)
	for _, rule := range []struct {
		name string
		run  func()
	}{
		{"aws", func() { awsKeyIDIn(text) }},
		{"keyed", func() { keyedValueIn(text, false) }},
		{"host", func() { hostCredentialIn(text) }},
		{"all", func() { credentialMatch(path, text) }},
	} {
		b.Run(rule.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			for range b.N {
				rule.run()
			}
		})
	}
}
