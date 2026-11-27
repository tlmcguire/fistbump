// Package catalogfeed fetches fixed-origin JSON catalogs for background refreshes.
package catalogfeed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Get allows up to three same-origin HTTPS redirects and rejects oversized responses.
func Get(ctx context.Context, client *http.Client, feedURL string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" || req.URL.Hostname() == "" || req.URL.User != nil {
		return nil, fmt.Errorf("catalog feed must be an HTTPS URL without credentials")
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("User-Agent", "Fistbump")
	if client == nil {
		client = http.DefaultClient
	}
	policy := *client
	policy.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > 3 || next.URL.Scheme != "https" || next.URL.User != nil ||
			!strings.EqualFold(next.URL.Host, req.URL.Host) {
			return http.ErrUseLastResponse
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(next, via)
		}
		return nil
	}
	res, err := policy.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog feed returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("catalog response exceeds %d bytes", maxBytes)
	}
	return body, nil
}
