package vcs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"gaal/internal/urlx"
)

const systemKnownHostsPath = "/etc/ssh/ssh_known_hosts"

var newSSHAgentAuth = gogitssh.NewSSHAgentAuth

var defaultSSHKeyNames = []string{
	"id_ed25519",
	"id_ecdsa",
	"id_rsa",
	"id_dsa",
}

type knownHostsConfig struct {
	files     []string
	writePath string
}

func sshAuthForURL(ctx context.Context, rawURL string) (transport.AuthMethod, error) {
	slog.DebugContext(ctx, "preparing git auth for URL", "url", urlx.Redact(rawURL))

	endpoint, err := transport.NewEndpoint(rawURL)
	if err != nil || endpoint.Protocol != "ssh" {
		return nil, nil
	}

	user := endpoint.User
	if user == "" {
		user = gogitssh.DefaultUsername
	}

	callback, err := acceptNewKnownHostsCallback(ctx)
	if err != nil {
		return nil, err
	}

	auth, err := sshPublicKeysAuth(ctx, user)
	if err != nil {
		return nil, err
	}
	auth.HostKeyCallback = callback
	return auth, nil
}

func sshPublicKeysAuth(ctx context.Context, user string) (*gogitssh.PublicKeysCallback, error) {
	slog.DebugContext(ctx, "creating SSH public key auth", "user", user)

	agentAuth, agentErr := newSSHAgentAuth(user)
	keyPaths, err := defaultSSHPrivateKeyPaths()
	if err != nil {
		return nil, err
	}
	fileSigners, fileErr := loadSSHPrivateKeySigners(ctx, keyPaths)
	if agentErr != nil && len(fileSigners) == 0 {
		if fileErr != nil {
			return nil, fmt.Errorf("creating SSH auth: agent unavailable (%v); loading SSH keys: %w", agentErr, fileErr)
		}
		return nil, fmt.Errorf("creating SSH auth: agent unavailable (%v) and no default SSH private keys found in ~/.ssh", agentErr)
	}

	return &gogitssh.PublicKeysCallback{
		User: user,
		Callback: func() ([]ssh.Signer, error) {
			slog.Debug("loading SSH signers for authentication", "file_signers", len(fileSigners), "has_agent", agentAuth != nil)

			signers := append([]ssh.Signer{}, fileSigners...)
			if agentAuth != nil && agentAuth.Callback != nil {
				agentSigners, err := agentAuth.Callback()
				if err != nil && len(signers) == 0 {
					return nil, fmt.Errorf("loading SSH agent signers: %w", err)
				}
				signers = append(signers, agentSigners...)
			}
			if len(signers) == 0 {
				return nil, fmt.Errorf("no SSH identities available; add a key to ssh-agent or create a default key in ~/.ssh")
			}
			return signers, nil
		},
	}, nil
}

func defaultSSHPrivateKeyPaths() ([]string, error) {
	slog.Debug("resolving default SSH private key paths")

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}
	paths := make([]string, 0, len(defaultSSHKeyNames))
	for _, name := range defaultSSHKeyNames {
		paths = append(paths, filepath.Join(home, ".ssh", name))
	}
	return paths, nil
}

func loadSSHPrivateKeySigners(ctx context.Context, paths []string) ([]ssh.Signer, error) {
	slog.DebugContext(ctx, "loading default SSH private keys", "count", len(paths))

	var signers []ssh.Signer
	var errs []error
	for _, path := range paths {
		signer, err := loadSSHPrivateKeySigner(path)
		if err == nil {
			signers = append(signers, signer)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return signers, errors.Join(errs...)
}

func loadSSHPrivateKeySigner(path string) (ssh.Signer, error) {
	slog.Debug("loading SSH private key", "path", shortPath(path))

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", shortPath(path), err)
	}
	return signer, nil
}

func acceptNewKnownHostsCallback(ctx context.Context) (ssh.HostKeyCallback, error) {
	slog.DebugContext(ctx, "creating accept-new known_hosts callback")

	cfg, err := prepareKnownHosts(ctx)
	if err != nil {
		return nil, err
	}

	callback, err := knownhosts.New(cfg.files...)
	if err != nil {
		return nil, fmt.Errorf("reading known_hosts: %w", err)
	}

	var mu sync.Mutex
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		mu.Lock()
		defer mu.Unlock()

		slog.Debug("checking SSH host key", "host", hostname, "known_hosts", shortPath(cfg.writePath))

		err := callback(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) || len(keyErr.Want) > 0 {
			return err
		}

		if err := appendKnownHost(cfg.writePath, hostname, key); err != nil {
			return err
		}
		callback, err = knownhosts.New(cfg.files...)
		if err != nil {
			return fmt.Errorf("reloading known_hosts: %w", err)
		}
		slog.Debug("accepted new SSH host key", "host", hostname, "known_hosts", shortPath(cfg.writePath))
		return nil
	}, nil
}

func prepareKnownHosts(ctx context.Context) (knownHostsConfig, error) {
	slog.DebugContext(ctx, "preparing known_hosts files")

	candidates, writePath, err := knownHostsCandidates()
	if err != nil {
		return knownHostsConfig{}, err
	}
	if err := ensureKnownHostsFile(writePath); err != nil {
		return knownHostsConfig{}, err
	}

	files := make([]string, 0, len(candidates))
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		} else if !os.IsNotExist(err) {
			return knownHostsConfig{}, fmt.Errorf("checking known_hosts %s: %w", shortPath(path), err)
		}
	}
	if len(files) == 0 {
		return knownHostsConfig{}, fmt.Errorf("no known_hosts files available")
	}

	return knownHostsConfig{files: files, writePath: writePath}, nil
}

func knownHostsCandidates() ([]string, string, error) {
	slog.Debug("resolving known_hosts candidates")

	if env := os.Getenv("SSH_KNOWN_HOSTS"); env != "" {
		files := nonEmptyPathList(env)
		if len(files) == 0 {
			return nil, "", fmt.Errorf("SSH_KNOWN_HOSTS does not contain a usable path")
		}
		return files, files[0], nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("resolving home directory: %w", err)
	}
	userKnownHosts := filepath.Join(home, ".ssh", "known_hosts")
	return []string{userKnownHosts, systemKnownHostsPath}, userKnownHosts, nil
}

func nonEmptyPathList(value string) []string {
	slog.Debug("splitting path list")

	parts := filepath.SplitList(value)
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			files = append(files, part)
		}
	}
	return files
}

func ensureKnownHostsFile(path string) error {
	slog.Debug("ensuring known_hosts file exists", "path", shortPath(path))

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating known_hosts directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("creating known_hosts file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing known_hosts file: %w", err)
	}
	return nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	slog.Debug("appending SSH host key", "host", hostname, "known_hosts", shortPath(path))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening known_hosts file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key)); err != nil {
		return fmt.Errorf("writing known_hosts entry: %w", err)
	}
	return nil
}
