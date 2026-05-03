package urlx

import "testing"

func TestValidateRemoteFetchURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty rejected", "", true},
		{"https ok", "https://github.com/owner/repo.json", false},
		{"https with creds ok", "https://u:p@github.com/repo.json", false},
		{"http loopback ok", "http://127.0.0.1:8080/x", false},
		{"http localhost ok", "http://localhost/x", false},
		{"http ipv6 loopback ok", "http://[::1]:9000/x", false},
		{"http public rejected", "http://example.com/x", true},
		{"http imds rejected (SSRF)", "http://169.254.169.254/latest/meta-data/", true},
		{"file rejected", "file:///etc/passwd", true},
		{"gopher rejected", "gopher://example.com/", true},
		{"dict rejected", "dict://example.com/", true},
		{"ssh rejected for fetch", "ssh://host/repo", true},
		{"missing scheme rejected", "github.com/owner/repo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemoteFetchURL(tt.in)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.in, err)
			}
		})
	}
}

func TestValidateRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty rejected", "", true},
		{"https ok", "https://github.com/owner/repo.git", false},
		{"ssh ok", "ssh://git@github.com:22/owner/repo.git", false},
		{"git ok", "git://github.com/owner/repo.git", false},
		{"http loopback ok", "http://127.0.0.1/repo.git", false},
		{"http public rejected", "http://example.com/repo.git", true},
		{"svn loopback ok", "svn://127.0.0.1/repo", false},
		{"svn localhost ok", "svn://localhost:3690/repo", false},
		{"svn public rejected", "svn://example.com/repo", true},
		{"bzr loopback ok", "bzr://127.0.0.1:4155/repo", false},
		{"bzr public rejected", "bzr://example.com/repo", true},
		{"file rejected", "file:///srv/repo.git", true},
		{"ftp rejected", "ftp://example.com/repo.git", true},
		{"scp-style git ok", "git@github.com:owner/repo.git", false},
		{"scp-style with user ok", "alice@host.example:foo/bar", false},
		{"plain local path ok", "/srv/local/repo", false},
		{"plain relative path ok", "./local/repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoURL(tt.in)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.in, err)
			}
		})
	}
}

func TestIsSCPStyleGitURL(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"git@github.com:owner/repo.git", true},
		{"alice@host:path", true},
		{"https://github.com/owner/repo.git", false},
		{"ssh://git@host/repo", false},
		{"@host:path", false},
		{"host:path", false},
		{"user@host", false},
		{"user@host.com/foo:bar", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isSCPStyleGitURL(tt.in); got != tt.want {
				t.Errorf("isSCPStyleGitURL(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
