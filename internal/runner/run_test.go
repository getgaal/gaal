package runner

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestRun_Success(t *testing.T) {
	// Use the "true" command that exits 0 on POSIX.
	if err := Run(context.Background(), "test label", "", "true"); err != nil {
		t.Fatalf("Run true: %v", err)
	}
}

func TestRun_Failure(t *testing.T) {
	err := Run(context.Background(), "test label", "", "false")
	if err == nil {
		t.Fatal("expected error from 'false' command")
	}
}

func TestRun_MissingBinary(t *testing.T) {
	err := Run(context.Background(), "test label", "", "this-binary-does-not-exist-gaal-test")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestRun_DebugMode_Success(t *testing.T) {
	// Enable debug level so runDebug path is taken.
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelDebug})
	_ = handler
	// Just verify that running with the default log level also works.
	if err := Run(context.Background(), "label", "", "echo", "debug mode test"); err != nil {
		t.Fatalf("Run in debug mode: %v", err)
	}
}

func TestRun_DebugLevel_UsesRunDebugPath(t *testing.T) {
	// Save and restore the default slog logger.
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	// Enable DEBUG level so the runDebug code path is taken.
	slog.SetDefault(slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	)))
	if err := Run(context.Background(), "debug test", "", "echo", "hello-debug"); err != nil {
		t.Fatalf("Run in debug mode: %v", err)
	}
}

func TestRun_DebugLevel_Failure(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	slog.SetDefault(slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	)))
	// "false" exits with code 1.
	err := Run(context.Background(), "debug fail", "", "false")
	if err == nil {
		t.Fatal("expected error from 'false' command in debug mode")
	}
}

func TestRun_OutputOnFailure(t *testing.T) {
	// A command that produces output AND fails covers the dumpCaptured slog path.
	err := Run(context.Background(), "output-fail", "", "sh", "-c", "echo error output; exit 1")
	if err == nil {
		t.Fatal("expected error from failing command with output")
	}
}
