package vcs

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeGitURL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"https with .git", "https://github.com/owner/repo.git", "github.com/owner/repo"},
		{"https without .git", "https://github.com/owner/repo", "github.com/owner/repo"},
		{"https trailing slash", "https://github.com/owner/repo/", "github.com/owner/repo"},
		{"ssh url", "ssh://git@github.com/owner/repo.git", "github.com/owner/repo"},
		{"scp-style", "git@github.com:owner/repo.git", "github.com/owner/repo"},
		{"scp-style no .git", "git@github.com:owner/repo", "github.com/owner/repo"},
		{"git protocol", "git://github.com/owner/repo.git", "github.com/owner/repo"},
		{"case difference", "HTTPS://GITHUB.com/Owner/Repo.git", "github.com/Owner/Repo"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeGitURL(tc.in)
			if got != tc.want {
				t.Errorf("normalizeGitURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestURLProtocolLabel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://github.com/o/r.git", "HTTPS"},
		{"http://localhost/r", "HTTP"},
		{"ssh://git@github.com/o/r", "SSH"},
		{"git@github.com:o/r.git", "SSH"},
		{"git://github.com/o/r", "git://"},
		{"file:///tmp/r", "file://"},
		{"/tmp/r", "local path"},
		{"", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := urlProtocolLabel(tc.in); got != tc.want {
				t.Errorf("urlProtocolLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRemoteURLMismatchError_Message(t *testing.T) {
	err := &RemoteURLMismatchError{
		Path:          "/home/u/repo",
		ConfiguredURL: "https://github.com/owner/repo.git",
		RemoteURL:     "git@github.com:owner/repo.git",
	}
	msg := err.Error()
	for _, want := range []string{
		"gaal.yaml URL is HTTPS",
		"remote at /home/u/repo is SSH",
		"https://github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
		"git remote set-url origin",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q\n  got: %s", want, msg)
		}
	}
}

func TestRemoteURLMismatchError_AsTarget(t *testing.T) {
	err := &RemoteURLMismatchError{Path: "p", ConfiguredURL: "a", RemoteURL: "b"}
	var target *RemoteURLMismatchError
	if !errors.As(err, &target) {
		t.Fatal("errors.As did not match RemoteURLMismatchError")
	}
}
