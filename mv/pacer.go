package mv

import "time"

// Pacer holds the throttle thresholds that a run learns from Master Vault.
// It lives for a whole run, so a later set starts at the pace that an earlier
// set settled on.
type Pacer struct {
	// Pace is the wait between requests. It goes up one second after each
	// throttle and does not come down.
	Pace time.Duration
	// MaxPace caps Pace.
	MaxPace time.Duration
	// Longest is the longest backoff wait so far. A new backoff starts there.
	Longest time.Duration
	// MaxBackoff caps one backoff wait.
	MaxBackoff time.Duration
}

// NewPacer returns a pacer with the polite start values.
func NewPacer() *Pacer {
	return &Pacer{
		Pace:       time.Second,
		MaxPace:    20 * time.Second,
		Longest:    2 * time.Second,
		MaxBackoff: time.Minute,
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
	return &backoff{p: p, wait: p.Longest}
}

// next returns the wait before the next retry, then doubles the wait up to
// MaxBackoff. It records the longest wait in the pacer.
func (b *backoff) next() time.Duration {
	w := b.wait
	b.p.Longest = max(b.p.Longest, w)
	b.wait = min(b.wait*2, b.p.MaxBackoff)
	return w
}
