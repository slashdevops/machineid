package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Timeouts for the two kinds of request.
const (
	lookupTimeout   = 20 * time.Second
	downloadTimeout = 10 * time.Minute
)

// Client talks to GitHub's web endpoints for a public repository.
//
// Nothing here uses the REST API: the newest tag is read from the redirect
// that github.com/<repo>/releases/latest returns, a tag's existence from the
// status of its release page, and assets from releases/download/<tag>/<name>.
// None of those carry the API's 60-requests-per-hour unauthenticated quota,
// and none need a token, which a public binary could not keep secret anyway.
type Client struct {
	// HTTP performs requests. Nil uses a client with the package's timeouts.
	HTTP *http.Client

	// BaseURL is the GitHub origin, "https://github.com" by default.
	// Tests point it at an in-memory server.
	BaseURL string

	// UserAgent identifies this tool to GitHub.
	UserAgent string

	// AllowHost decides which hosts a download may be redirected to.
	// Nil allows github.com and *.githubusercontent.com.
	AllowHost func(host string) bool

	// Repo overrides Repository, for tests.
	Repo string
}

// NewClient returns a client identifying itself with the running version.
func NewClient(version string) *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: downloadTimeout},
		BaseURL:   "https://github.com",
		UserAgent: fmt.Sprintf("%s/%s (+https://github.com/%s)", ToolName, version, Repository),
	}
}

func (c *Client) repo() string {
	if c.Repo != "" {
		return c.Repo
	}

	return Repository
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}

	return "https://github.com"
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}

	return http.DefaultClient
}

func (c *Client) allowHost(host string) bool {
	if c.AllowHost != nil {
		return c.AllowHost(host)
	}

	host = strings.ToLower(host)
	if h, _, err := splitHostPort(host); err == nil {
		host = h
	}

	return host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

// splitHostPort tolerates a missing port.
func splitHostPort(hostport string) (string, string, error) {
	i := strings.LastIndexByte(hostport, ':')
	if i < 0 || strings.Contains(hostport[i:], "]") {
		return hostport, "", errors.New("no port")
	}

	return hostport[:i], hostport[i+1:], nil
}

// LatestURL is the page that redirects to the newest release.
func (c *Client) LatestURL() string {
	return c.base() + "/" + c.repo() + "/releases/latest"
}

// TagURL is a release's page.
func (c *Client) TagURL(tag string) string {
	return c.base() + "/" + c.repo() + "/releases/tag/" + url.PathEscape(tag)
}

// AssetURL is the download URL of one asset of one release.
func (c *Client) AssetURL(tag, name string) string {
	return c.base() + "/" + c.repo() + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
}

// Latest returns the tag GitHub marks as the latest release.
//
// That is deliberately not "the highest tag": which release users should move
// to is a decision the release process makes when it sets the marker.
func (c *Client) Latest(ctx context.Context) (string, error) {
	resp, err := c.head(ctx, c.LatestURL())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := c.rateLimited(resp); err != nil {
		return "", err
	}

	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("%s answered HTTP %d instead of redirecting to the latest release", c.LatestURL(), resp.StatusCode)
	}

	loc, err := resp.Location()
	if err != nil {
		return "", fmt.Errorf("%s redirected without a Location header", c.LatestURL())
	}

	tag, ok := tagFromReleasePath(loc.Path)
	if !ok {
		return "", fmt.Errorf("unexpected redirect target %s", loc)
	}

	return tag, nil
}

// tagFromReleasePath extracts <tag> from ".../releases/tag/<tag>".
func tagFromReleasePath(p string) (string, bool) {
	const marker = "/releases/tag/"
	_, after, ok := strings.Cut(p, marker)
	if !ok {
		return "", false
	}

	tag, err := url.PathUnescape(after)
	if err != nil || tag == "" || strings.Contains(tag, "/") {
		return "", false
	}

	return tag, true
}

// TagExists reports whether a release page exists for tag.
func (c *Client) TagExists(ctx context.Context, tag string) (bool, error) {
	return c.exists(ctx, c.TagURL(tag))
}

// AssetExists reports whether a release carries the named asset, without
// downloading it. The download URL answers with a redirect when the asset
// exists and 404 when it does not.
func (c *Client) AssetExists(ctx context.Context, tag, name string) (bool, error) {
	return c.exists(ctx, c.AssetURL(tag, name))
}

func (c *Client) exists(ctx context.Context, u string) (bool, error) {
	resp, err := c.head(ctx, u)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if err := c.rateLimited(resp); err != nil {
		return false, err
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		return true, nil
	default:
		return false, fmt.Errorf("%s answered HTTP %d", u, resp.StatusCode)
	}
}

// head performs a HEAD request without following redirects; the caller reads
// the status and Location itself.
func (c *Client) head(ctx context.Context, u string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	client := *c.httpClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	resp, err := client.Do(req)
	if err != nil {
		return nil, &NoConnectivityError{URL: u, Err: err}
	}

	return resp, nil
}

// rateLimited turns a 429 or a rate-limit 403 into a RateLimitedError.
func (c *Client) rateLimited(resp *http.Response) error {
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusForbidden {
		return nil
	}

	retryAt := time.Now().Add(time.Hour)
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			retryAt = time.Now().Add(time.Duration(secs) * time.Second)
		} else if t, err := http.ParseTime(ra); err == nil {
			retryAt = t
		}
	} else if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if secs, err := strconv.ParseInt(reset, 10, 64); err == nil {
			retryAt = time.Unix(secs, 0)
		}
	}

	return &RateLimitedError{Status: resp.StatusCode, RetryAt: retryAt}
}

// Download fetches one asset of one release into dest, following redirects
// only to allowed hosts and streaming to disk.
func (c *Client) Download(ctx context.Context, tag, name, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	u := c.AssetURL(tag, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	client := *c.httpClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if !c.allowHost(req.URL.Host) {
			return fmt.Errorf("refusing to follow a redirect to %s", req.URL.Host)
		}

		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &NoConnectivityError{URL: u, Err: err}
	}
	defer resp.Body.Close()

	if err := c.rateLimited(resp); err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return &AssetNotFoundError{Asset: name, Tag: tag}
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: HTTP %d", name, resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}

	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()

	if copyErr != nil {
		return errors.Join(fmt.Errorf("downloading %s: %w", name, copyErr), closeErr, os.Remove(dest))
	}

	return closeErr
}
