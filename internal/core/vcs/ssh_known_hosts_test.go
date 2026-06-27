package vcs

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	if !publicKey.Equal(privateKey.Public()) {
		t.Fatal("generated public key does not match private key")
	}
	return signer.PublicKey()
}

func testPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	data, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data})
}

func TestKnownHostsCandidates_DefaultUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_KNOWN_HOSTS", "")

	files, writePath, err := knownHostsCandidates()
	if err != nil {
		t.Fatalf("knownHostsCandidates: %v", err)
	}

	want := filepath.Join(home, ".ssh", "known_hosts")
	if writePath != want {
		t.Fatalf("writePath = %q, want %q", writePath, want)
	}
	if len(files) != 2 || files[0] != want || files[1] != systemKnownHostsPath {
		t.Fatalf("files = %#v", files)
	}
}

func TestKnownHostsCandidates_UsesSSHKnownHostsEnv(t *testing.T) {
	first := filepath.Join(t.TempDir(), "known_hosts")
	second := filepath.Join(t.TempDir(), "extra_known_hosts")
	t.Setenv("SSH_KNOWN_HOSTS", first+string(os.PathListSeparator)+second)

	files, writePath, err := knownHostsCandidates()
	if err != nil {
		t.Fatalf("knownHostsCandidates: %v", err)
	}

	if writePath != first {
		t.Fatalf("writePath = %q, want %q", writePath, first)
	}
	if len(files) != 2 || files[0] != first || files[1] != second {
		t.Fatalf("files = %#v", files)
	}
}

func TestPrepareKnownHosts_CreatesUserFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_KNOWN_HOSTS", "")

	cfg, err := prepareKnownHosts(context.Background())
	if err != nil {
		t.Fatalf("prepareKnownHosts: %v", err)
	}

	want := filepath.Join(home, ".ssh", "known_hosts")
	if cfg.writePath != want {
		t.Fatalf("writePath = %q, want %q", cfg.writePath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("known_hosts was not created: %v", err)
	}
	if len(cfg.files) == 0 || cfg.files[0] != want {
		t.Fatalf("files = %#v", cfg.files)
	}
}

func TestAcceptNewKnownHostsCallback_AppendsUnknownHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("SSH_KNOWN_HOSTS", path)

	callback, err := acceptNewKnownHostsCallback(context.Background())
	if err != nil {
		t.Fatalf("acceptNewKnownHostsCallback: %v", err)
	}

	key := testPublicKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	if err := callback("example.com:22", remote, key); err != nil {
		t.Fatalf("callback unknown host: %v", err)
	}
	if err := callback("example.com:22", remote, key); err != nil {
		t.Fatalf("callback appended host: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := strings.Count(string(data), "example.com "); got != 1 {
		t.Fatalf("known_hosts entry count = %d, want 1\n%s", got, data)
	}
}

func TestAcceptNewKnownHostsCallback_RejectsChangedHostKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	oldKey := testPublicKey(t)
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{"example.com:22"}, oldKey)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SSH_KNOWN_HOSTS", path)

	callback, err := acceptNewKnownHostsCallback(context.Background())
	if err != nil {
		t.Fatalf("acceptNewKnownHostsCallback: %v", err)
	}

	newKey := testPublicKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	err = callback("example.com:22", remote, newKey)
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) || len(keyErr.Want) == 0 {
		t.Fatalf("expected known_hosts key mismatch, got %T: %v", err, err)
	}
}

func TestSSHAuthForURL_NonSSHReturnsNil(t *testing.T) {
	auth, err := sshAuthForURL(context.Background(), "https://github.com/getgaal/gaal.git")
	if err != nil {
		t.Fatalf("sshAuthForURL: %v", err)
	}
	if auth != nil {
		t.Fatalf("auth = %T, want nil", auth)
	}
}

func TestSSHAuthForURL_SSHUsesGitUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("SSH_KNOWN_HOSTS", path)

	oldNewSSHAgentAuth := newSSHAgentAuth
	t.Cleanup(func() { newSSHAgentAuth = oldNewSSHAgentAuth })
	var gotUser string
	newSSHAgentAuth = func(user string) (*gogitssh.PublicKeysCallback, error) {
		gotUser = user
		return &gogitssh.PublicKeysCallback{User: user}, nil
	}

	auth, err := sshAuthForURL(context.Background(), "git@github.com:owner/repo.git")
	if err != nil {
		t.Fatalf("sshAuthForURL: %v", err)
	}
	if gotUser != gogitssh.DefaultUsername {
		t.Fatalf("newSSHAgentAuth user = %q, want %q", gotUser, gogitssh.DefaultUsername)
	}
	if got := auth.(*gogitssh.PublicKeysCallback).User; got != gogitssh.DefaultUsername {
		t.Fatalf("User = %q, want %q", got, gogitssh.DefaultUsername)
	}
}

func TestDefaultSSHPrivateKeyPaths_UsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	paths, err := defaultSSHPrivateKeyPaths()
	if err != nil {
		t.Fatalf("defaultSSHPrivateKeyPaths: %v", err)
	}
	if len(paths) != len(defaultSSHKeyNames) {
		t.Fatalf("len(paths) = %d, want %d", len(paths), len(defaultSSHKeyNames))
	}
	if paths[0] != filepath.Join(home, ".ssh", defaultSSHKeyNames[0]) {
		t.Fatalf("first key path = %q", paths[0])
	}
}

func TestLoadSSHPrivateKeySigners_LoadsDefaultKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, testPrivateKeyPEM(t), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	signers, err := loadSSHPrivateKeySigners(context.Background(), []string{
		keyPath,
		filepath.Join(dir, "missing"),
	})
	if err != nil {
		t.Fatalf("loadSSHPrivateKeySigners: %v", err)
	}
	if len(signers) != 1 {
		t.Fatalf("len(signers) = %d, want 1", len(signers))
	}
}

func TestSSHPublicKeysAuth_UsesDefaultKeyWhenAgentUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), testPrivateKeyPEM(t), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	oldNewSSHAgentAuth := newSSHAgentAuth
	t.Cleanup(func() { newSSHAgentAuth = oldNewSSHAgentAuth })
	newSSHAgentAuth = func(user string) (*gogitssh.PublicKeysCallback, error) {
		return nil, fmt.Errorf("agent unavailable for %s", user)
	}

	auth, err := sshPublicKeysAuth(context.Background(), gogitssh.DefaultUsername)
	if err != nil {
		t.Fatalf("sshPublicKeysAuth: %v", err)
	}
	signers, err := auth.Callback()
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if len(signers) != 1 {
		t.Fatalf("len(signers) = %d, want 1", len(signers))
	}
}

func TestSSHPublicKeysAuth_ErrorsWithoutAgentOrKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldNewSSHAgentAuth := newSSHAgentAuth
	t.Cleanup(func() { newSSHAgentAuth = oldNewSSHAgentAuth })
	newSSHAgentAuth = func(user string) (*gogitssh.PublicKeysCallback, error) {
		return nil, fmt.Errorf("agent unavailable for %s", user)
	}

	_, err := sshPublicKeysAuth(context.Background(), gogitssh.DefaultUsername)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no default SSH private keys") {
		t.Fatalf("error = %v", err)
	}
}
