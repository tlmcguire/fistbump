package catalogfeed

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type feedTransport func(*http.Request) (*http.Response, error)

func (f feedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetChecksOriginAndBodySize(t *testing.T) {
	const origin = "https://feed.example"
	client := &http.Client{Transport: feedTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "feed.example" {
			t.Errorf("unexpected catalog destination: %s", req.URL.Host)
		}
		res := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
		var body string
		switch req.URL.Path {
		case "/same":
			res.StatusCode = http.StatusFound
			res.Header.Set("Location", "/catalog")
		case "/other":
			res.StatusCode = http.StatusFound
			res.Header.Set("Location", "https://other.example/catalog")
		case "/loop":
			res.StatusCode = http.StatusFound
			res.Header.Set("Location", "/loop")
		case "/catalog":
			body = "{}"
		case "/large":
			body = "{} "
		}
		res.Body = io.NopCloser(strings.NewReader(body))
		return res, nil
	})}

	ctx := context.Background()
	body, err := Get(ctx, client, origin+"/same", 2)
	if err != nil || string(body) != "{}" {
		t.Fatalf("same-origin feed: %q, %v", body, err)
	}
	for _, url := range []string{origin + "/other", origin + "/loop", origin + "/large", "http://feed.example/catalog"} {
		if _, err := Get(ctx, client, url, 2); err == nil {
			t.Errorf("%s: expected rejection", url)
		}
	}
}
