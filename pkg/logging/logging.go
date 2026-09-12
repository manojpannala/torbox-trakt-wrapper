// Package logging provides the --verbose file logger.
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
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

// credentialParam matches a credential carried in a query string. TorBox's
// download-link endpoints put the api key in `token=`, so a logged path or a
// *url.Error carries it.
var credentialParam = regexp.MustCompile(`(?i)([?&](?:token|api_key|apikey|key|secret|auth|access_token|refresh_token|client_secret)=)[^&\s"]*`)

// Redact blanks credential query parameters in s, which may be a URL, a path,
// or an error message that embeds one.
func Redact(s string) string {
	return credentialParam.ReplaceAllString(s, "${1}"+redacted)
}

func redactSecrets(_ []string, a slog.Attr) slog.Attr {
	if secretKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, redacted)
	}
	return a
}
