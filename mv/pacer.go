package mv

import "time"

// Pacer holds the throttle settings of a run. It lives for a whole run, so a
// later set starts at the pace that an earlier set settled on.
type Pacer struct {
	// Pace is the wait between requests. It goes up one second after each
	// throttle and does not come down.
	Pace time.Duration
	// MaxPace caps Pace.
	MaxPace time.Duration
	// FirstBackoff is the first wait after a throttle. Each throttle in a row
	// doubles the wait, up to MaxBackoff. Master Vault sends no Retry-After
	// header, and a retry every minute kept a block for hours.
	FirstBackoff time.Duration
	// MaxBackoff caps one backoff wait.
	MaxBackoff time.Duration
	// Longest is the longest backoff wait of the run, for the logs.
	Longest time.Duration
}

// NewPacer returns a pacer with the polite start values. A pace of about 10
// seconds read 240 feed pages an hour for 8 hours without a throttle. A pace
// of 1 second got a throttle after 23 pages.
func NewPacer() *Pacer {
	return &Pacer{
		Pace:         10 * time.Second,
		MaxPace:      time.Minute,
		FirstBackoff: 10 * time.Second,
		MaxBackoff:   time.Hour,
	}
}

// Throttled increases the pace by one second, up to MaxPace.
func (p *Pacer) Throttled() {
	p.Pace = min(p.Pace+time.Second, p.MaxPace)
}

// backoff is the retry state of one request.
type backoff struct {
	p    *Pacer
	wait time.Duration
}

func (p *Pacer) backoff() *backoff {
	return &backoff{p: p, wait: p.FirstBackoff}
}

// next returns the wait before the next retry, then doubles the wait up to
// MaxBackoff. It records the longest wait in the pacer.
func (b *backoff) next() time.Duration {
	w := b.wait
	b.p.Longest = max(b.p.Longest, w)
	b.wait = min(b.wait*2, b.p.MaxBackoff)
	return w
}
