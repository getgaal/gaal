package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"gaal/internal/logger"
)

// Run executes name with args in dir, showing progress appropriate to the
// current log level.
//
//   - DEBUG : stdout and stderr of the subprocess are streamed line-by-line
//     as slog.Debug records — full visibility for troubleshooting.
//   - INFO  : output is captured silently; a TTY spinner shows the label.
//     On failure the captured output is logged via slog.Error.
func Run(ctx context.Context, label, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec
	cmd.Dir = dir

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		return runDebug(ctx, cmd, name)
	}

	return runInfo(ctx, cmd, name, label)
}

// runDebug captures subprocess stdout+stderr into a buffer, then logs each
// line at DEBUG level after the command exits.
// Using bytes.Buffer (instead of io.Pipe) avoids any pipe-synchronisation
// deadlock: the subprocess writes freely; we scan the result once it's done.
func runDebug(ctx context.Context, cmd *exec.Cmd, cmdName string) error {
	var captured bytes.Buffer
	cmd.Stdout = &captured
	cmd.Stderr = &captured

	err := cmd.Run()

	scanner := bufio.NewScanner(&captured)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			slog.DebugContext(ctx, line, "cmd", cmdName)
		}
	}

	return err
}

// runInfo captures output silently and shows a spinner (TTY only).
// On error, the captured output is logged line-by-line via slog.Error.
func runInfo(ctx context.Context, cmd *exec.Cmd, cmdName, label string) error {
	var captured bytes.Buffer
	cmd.Stdout = &captured
	cmd.Stderr = &captured

	sp := logger.StartSpinner(os.Stderr, label)

	err := cmd.Run()

	if err != nil {
		sp.Done(false, label)
		dumpCaptured(ctx, captured.String(), cmdName)
		return fmt.Errorf("%w", err)
	}

	sp.Done(true, label)
	return nil
}

// dumpCaptured logs each non-empty line from subprocess output as slog.Error.
func dumpCaptured(ctx context.Context, output, cmdName string) {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) != "" {
			slog.ErrorContext(ctx, line, "cmd", cmdName)
		}
	}
}
