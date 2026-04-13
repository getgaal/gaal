// Package installscript hosts integration tests for scripts/install.sh.
//
// These tests spin up a local HTTP server that mimics GitHub Releases and
// exec the shell script against it. The script honors the (test-only)
// GAAL_INSTALL_BASE_URL env var so the test can point it at the local
// server without touching github.com.
package installscript
