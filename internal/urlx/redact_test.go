package urlx

import "testing"

func TestRedact(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no userinfo", "https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"user and password", "https://alice:hunter2@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"user only", "https://alice@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"token-as-password", "https://oauth2:ghp_xxxx@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"ssh with creds", "ssh://user:token@host:22/path", "ssh://host:22/path"},
		{"http with port and creds", "http://u:p@127.0.0.1:8080/x", "http://127.0.0.1:8080/x"},
		{"scp-style git (unchanged)", "git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"local path (unchanged)", "/home/alice/skills", "/home/alice/skills"},
		{"unparseable (unchanged)", "://broken", "://broken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.in)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSlogURL(t *testing.T) {
	a := SlogURL("https://alice:secret@example.com/x")
	if a.Key != "url" {
		t.Errorf("key = %q, want %q", a.Key, "url")
	}
	if got := a.Value.String(); got != "https://example.com/x" {
		t.Errorf("value = %q, want %q", got, "https://example.com/x")
	}
}
