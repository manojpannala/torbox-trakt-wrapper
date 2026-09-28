package stream

import "time"

// WithMinRenewGap lets tests renew back to back.
func WithMinRenewGap(d time.Duration) Option {
	return func(p *Proxy) { p.minRenewGap = d }
}
