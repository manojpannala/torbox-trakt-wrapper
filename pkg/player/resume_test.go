package player_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
)

// capturedArgs runs a stub in place of mpv and returns the argv it was given.
func capturedArgs(t *testing.T, media player.MediaStream) []string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "argv")
	stub := filepath.Join(dir, "fake-mpv")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > '%s'\n", argsFile)
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o755)) // #nosec G306 -- the stub must be executable

	p := player.NewMPVPlayer(player.WithExecutable(stub), player.WithIPCEnabled(false))
	session, err := p.Play(context.Background(), media)
	require.NoError(t, err)
	require.NoError(t, session.Wait())

	raw, err := os.ReadFile(argsFile) // #nosec G304
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func TestMPVPlayer_ResumePositionIsSentAsAPercentage(t *testing.T) {
	args := capturedArgs(t, player.MediaStream{
		URL:             "https://example.invalid/a.mkv",
		ResumeAtPercent: 41,
	})

	assert.Contains(t, args, "--start=41.0%",
		"Trakt stores a percentage; emitting it as seconds seeks to 41s, not 41%")
}

func TestMPVPlayer_NoResumePositionMeansNoStartFlag(t *testing.T) {
	args := capturedArgs(t, player.MediaStream{URL: "https://example.invalid/a.mkv"})

	for _, a := range args {
		assert.False(t, strings.HasPrefix(a, "--start="), "unexpected %q", a)
	}
}

func TestMPVPlayer_LogsArgvWithoutTheSignedURL(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "fake-mpv")
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755)) // #nosec G306 -- the stub must be executable

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	p := player.NewMPVPlayer(
		player.WithExecutable(stub),
		player.WithIPCEnabled(false),
		player.WithLogger(log),
	)
	session, err := p.Play(context.Background(), player.MediaStream{
		URL:             "https://store.torbox.app/file.mkv?token=sig-abc123&expires=99",
		ResumeAtPercent: 41,
	})
	require.NoError(t, err)
	require.NoError(t, session.Wait())

	out := buf.String()
	assert.Contains(t, out, "--start=41.0%", "the launch flags are the point of the log")
	assert.NotContains(t, out, "sig-abc123", "a signed stream URL must never reach the log")
	assert.Contains(t, out, "store.torbox.app", "the host is useful and not sensitive")
}
