// Package mv is the Master Vault client. It reads the deck feed and single
// decks, and it slows down when Master Vault throttles it.
//
// The client returns the response bytes as they are. It decodes only the
// fields it needs to page the feed and to split a response into decks and
// linked objects (see Page).
package mv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Master Vault decks API.
const DefaultBaseURL = "https://www.keyforgegame.com/api/decks/"

// PageSize is the largest page that the feed returns.
const PageSize = 10

// ErrPastLimit is returned when the feed refuses a page past its limit of
// about 50,000 decks for one query (HTTP 403).
var ErrPastLimit = errors.New("mv: page is past the feed limit")

// ErrNotFound is returned for a deck or page that does not exist (HTTP 404).
var ErrNotFound = errors.New("mv: not found")

// Locales maps each Master Vault language to its Accept-Language value.
var Locales = map[string]string{
	"en":     "en-us",
	"de":     "de-de",
	"es":     "es-es",
	"fr":     "fr-fr",
	"it":     "it-it",
	"pl":     "pl-pl",
	"pt":     "pt-pt",
	"ru":     "ru-ru",
	"th":     "th-th",
	"vi":     "vi-vi",
	"zhhans": "zh-hans",
	"zhhant": "zh-hant",
}

// LocaleOrder lists the languages with English first.
var LocaleOrder = []string{
	"en", "de", "es", "fr", "it", "pl", "pt", "ru", "th", "vi", "zhhans", "zhhant",
}

// Client reads Master Vault. One client paces every request, so use one
// client for a whole run.
type Client struct {
	// BaseURL is the decks API. A proxy can replace it.
	BaseURL string
	// HTTP is the HTTP client.
	HTTP *http.Client
	// Pacer sets the wait between requests and the backoff after a throttle.
	Pacer *Pacer
	// Log receives progress and retry messages.
	Log *slog.Logger
	// UserAgent is sent with each request.
	UserAgent string

	last time.Time
}

// New returns a client with the default settings. It opens a new connection
// for each request: pooled connections go half-dead under throttling and then
// hang the next request until the timeout.
func New(log *slog.Logger) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives:   true,
				TLSHandshakeTimeout: 15 * time.Second,
			},
		},
		Pacer:     NewPacer(),
		Log:       log,
		UserAgent: "keyforge-data/1.0 (+https://github.com/dmikalova/keyforge-data)",
	}
}

// FeedQuery selects one page of the deck feed.
type FeedQuery struct {
	Expansion int
	Page      int
	// Ordering is "date" (oldest first) or "-date" (newest first).
	Ordering string
	// Houses filters by house names, if not empty.
	Houses []string
	// Locale is a key of Locales. Empty means English.
	Locale string
}

// Feed reads one page of the deck feed with the linked cards.
func (c *Client) Feed(ctx context.Context, q FeedQuery) (*Page, error) {
	v := url.Values{}
	v.Set("page", strconv.Itoa(q.Page))
	v.Set("page_size", strconv.Itoa(PageSize))
	v.Set("links", "cards")
	if q.Expansion != 0 {
		v.Set("expansion", strconv.Itoa(q.Expansion))
	}
	ordering := q.Ordering
	if ordering == "" {
		ordering = "date"
	}
	v.Set("ordering", ordering)
	if len(q.Houses) > 0 {
		v.Set("houses", strings.Join(q.Houses, ","))
	}
	body, err := c.get(ctx, c.BaseURL+"?"+v.Encode(), q.Locale)
	if err != nil {
		return nil, err
	}
	return ParsePage(body)
}

// Deck reads one deck with its linked cards, in one language.
func (c *Client) Deck(ctx context.Context, id, locale string) (*Page, error) {
	body, err := c.get(ctx, c.BaseURL+url.PathEscape(id)+"/?links=cards", locale)
	if err != nil {
		return nil, err
	}
	return ParseDeck(body)
}

// get sends one paced request. It retries a throttle (429), a gateway error
// (5xx), and a network error until it gets an answer, with a backoff from the
// pacer.
func (c *Client) get(ctx context.Context, reqURL, locale string) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	b := c.Pacer.backoff()
	for {
		body, status, err := c.do(ctx, reqURL, locale)
		c.last = time.Now()
		switch {
		case err != nil && ctx.Err() != nil:
			return nil, ctx.Err()
		case err != nil:
			c.Log.Warn("request failed", "url", reqURL, "err", err, "wait", b.wait)
		case status == http.StatusOK:
			return body, nil
		case status == http.StatusNotFound:
			return nil, ErrNotFound
		case status == http.StatusForbidden:
			return nil, ErrPastLimit
		case status == http.StatusTooManyRequests:
			c.Pacer.Throttled()
			c.Log.Warn("throttled", "url", reqURL, "wait", b.wait, "pace", c.Pacer.Pace)
		case status >= 500:
			c.Log.Warn("gateway error", "url", reqURL, "status", status, "wait", b.wait)
		default:
			return nil, fmt.Errorf(
				"mv: status %d for %s: %s",
				status, reqURL, string(bytes.TrimSpace(head(body, 300))),
			)
		}
		if err := sleep(ctx, b.next()); err != nil {
			return nil, err
		}
	}
}

// do sends one request and reads the whole body.
func (c *Client) do(ctx context.Context, reqURL, locale string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if l, ok := Locales[locale]; ok {
		req.Header.Set("Accept-Language", l)
	} else {
		req.Header.Set("Accept-Language", Locales["en"])
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// wait keeps the pace between requests.
func (c *Client) wait(ctx context.Context) error {
	if c.last.IsZero() {
		return nil
	}
	return sleep(ctx, time.Until(c.last.Add(c.Pacer.Pace)))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func head(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}
