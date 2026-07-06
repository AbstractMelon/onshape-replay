// Package onshape wraps the Onshape REST API. No other package in this
// application calls Onshape directly. All calls require a valid OAuth2 bearer
// token. The client handles one automatic token refresh on 401.
package onshape

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

const baseURL = "https://cad.onshape.com/api/v6"

// TokenRefresher is a function that can exchange a refresh token for a new
// access token. It is injected from the auth package to avoid a circular
// dependency.
type TokenRefresher func(ctx context.Context) (newAccessToken string, err error)

// Client wraps the Onshape REST API with bearer-token authentication.
type Client struct {
	httpClient     *http.Client
	log            *slog.Logger
	tokenRefresher TokenRefresher
	// getAccessToken returns the current access token from the session.
	getAccessToken func() string
}

// NewClient creates a new Onshape API client.
// getAccessToken must return the session's current access token on each call.
// refresher will be invoked at most once per request on a 401 to refresh the token.
func NewClient(log *slog.Logger, getAccessToken func() string, refresher TokenRefresher) *Client {
	return &Client{
		httpClient:     &http.Client{},
		log:            log,
		getAccessToken: getAccessToken,
		tokenRefresher: refresher,
	}
}

// get performs an authenticated GET request and decodes the JSON body into dst.
func (c *Client) get(ctx context.Context, path string, query url.Values, dst any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, dst)
}

// postJSON performs an authenticated POST request with a JSON body.
func (c *Client) postJSON(ctx context.Context, path string, body any, dst any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	return c.do(ctx, http.MethodPost, path, nil, strings.NewReader(string(b)), dst)
}

// getRaw performs an authenticated GET and returns the raw response body bytes.
func (c *Client) getRaw(ctx context.Context, path string, query url.Values) ([]byte, string, error) {
	u := baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAccessToken())
	req.Header.Set("Accept", "application/json;charset=UTF-8;qs=0.09")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized && c.tokenRefresher != nil {
		newToken, rErr := c.tokenRefresher(ctx)
		if rErr != nil {
			return nil, "", fmt.Errorf("token refresh: %w", rErr)
		}
		_ = newToken
		// Rebuild request with new token.
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req2.Header.Set("Authorization", "Bearer "+c.getAccessToken())
		req2.Header.Set("Accept", "application/json;charset=UTF-8;qs=0.09")
		resp2, err2 := c.httpClient.Do(req2)
		if err2 != nil {
			return nil, "", err2
		}
		defer resp2.Body.Close()
		if resp2.StatusCode >= 400 {
			b, _ := io.ReadAll(resp2.Body)
			return nil, "", fmt.Errorf("Onshape API %s: %d %s", path, resp2.StatusCode, string(b))
		}
		raw, e := io.ReadAll(resp2.Body)
		return raw, resp2.Header.Get("Content-Type"), e
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("Onshape API %s: %d %s", path, resp.StatusCode, string(b))
	}
	raw, e := io.ReadAll(resp.Body)
	return raw, resp.Header.Get("Content-Type"), e
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader, dst any) error {
	u := baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	// Buffer the body so we can retry on 401 without consuming it.
	var bodyBuf []byte
	if body != nil {
		var err error
		bodyBuf, err = io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(bodyBuf))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.getAccessToken())
	if body != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8;qs=0.09")
	}
	req.Header.Set("Accept", "application/json;charset=UTF-8;qs=0.09")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized && c.tokenRefresher != nil {
		_, rErr := c.tokenRefresher(ctx)
		if rErr != nil {
			return fmt.Errorf("token refresh: %w", rErr)
		}
		// Rebuild with refreshed token and re-read from buffer.
		req2, _ := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(bodyBuf))
		req2.Header.Set("Authorization", "Bearer "+c.getAccessToken())
		if body != nil {
			req2.Header.Set("Content-Type", "application/json;charset=UTF-8;qs=0.09")
		}
		req2.Header.Set("Accept", "application/json;charset=UTF-8;qs=0.09")
		resp2, err2 := c.httpClient.Do(req2)
		if err2 != nil {
			return err2
		}
		defer resp2.Body.Close()
		return decodeResponse(path, resp2, dst)
	}

	return decodeResponse(path, resp, dst)
}

func decodeResponse(path string, resp *http.Response, dst any) error {
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Onshape API %s: %d %s", path, resp.StatusCode, string(b))
	}
	if dst == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode response from %s: %w", path, err)
	}
	return nil
}
