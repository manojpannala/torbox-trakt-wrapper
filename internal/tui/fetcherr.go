package tui

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type fetchErrKind int

const (
	fetchErrOther fetchErrKind = iota
	fetchErrOffline
	fetchErrAuthRejected
)

const torboxKeyRejectedStatus = "TorBox rejected your API key — run: tt-wrapper auth torbox <key>"

// classifyFetchErr sorts a failed fetch by what the user can do about it: a
// rejected credential needs a new one, and a request that never got an answer
// (or found the service down) means waiting for the network.
func classifyFetchErr(err error) fetchErrKind {
	if torbox.IsUnauthorized(err) || trakt.IsUnauthorized(err) {
		return fetchErrAuthRejected
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		return fetchErrOffline
	}
	var tbErr *torbox.APIError
	if errors.As(err, &tbErr) && tbErr.StatusCode >= http.StatusInternalServerError {
		return fetchErrOffline
	}
	var trErr *trakt.APIError
	if errors.As(err, &trErr) && trErr.StatusCode >= http.StatusInternalServerError {
		return fetchErrOffline
	}
	return fetchErrOther
}
