// Package logging provides the --verbose file logger.
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

const redacted = "REDACTED"

// secretKeys are attribute names whose values never reach the log. Call sites
// are expected not to pass secrets at all; this is the backstop.
var secretKeys = map[string]bool{
	"api_key":       true,
	"apikey":        true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"client_secret": true,
	"authorization": true,
	"password":      true,
	"secret":        true,
}

// New returns a logger writing debug records to path. When verbose is false
// nothing is written and no file is created.
func New(verbose bool, path string) (*slog.Logger, func() error, error) {
	if !verbose {
		return slog.New(slog.DiscardHandler), func() error { return nil }, nil
	}

	if err := config.EnsureSecureDir(filepath.Dir(path)); err != nil {
		return nil, nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePermission) // #nosec G304 -- the path is ours, from config.GetLogFile
	if err != nil {
		return nil, nil, err
	}

	handler := slog.NewTextHandler(file, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: redactSecrets,
	})
	return slog.New(handler), file.Close, nil
}

func redactSecrets(_ []string, a slog.Attr) slog.Attr {
	if secretKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, redacted)
	}
	return a
}
