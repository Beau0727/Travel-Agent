package amap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"travel-agent-go/internal/config"
)

// Client centralizes Web Service calls to AMap.
// Weather/geocoding/POI use v3 endpoints, while route planning 2.0 uses v5.
type Client struct {
	apiKey string
	baseV3 string
	baseV5 string
	client *http.Client
}

func NewClient(cfg config.Config) *Client {
	timeout := cfg.AmapTimeoutSeconds
	if timeout <= 0 {
		timeout = 15
	}

	baseV3 := strings.TrimRight(cfg.AmapBaseV3URL, "/")
	if baseV3 == "" {
		baseV3 = strings.TrimRight(cfg.AmapBaseURL, "/")
	}
	if baseV3 == "" {
		baseV3 = "https://restapi.amap.com/v3"
	}

	baseV5 := strings.TrimRight(cfg.AmapBaseV5URL, "/")
	if baseV5 == "" {
		baseV5 = "https://restapi.amap.com/v5"
	}

	return &Client{
		apiKey: strings.TrimSpace(cfg.AmapAPIKey),
		baseV3: baseV3,
		baseV5: baseV5,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && strings.TrimSpace(c.apiKey) != ""
}

func (c *Client) GetV3(ctx context.Context, path string, values url.Values, out any) error {
	return c.getJSON(ctx, c.baseV3, path, values, out)
}

func (c *Client) GetV5(ctx context.Context, path string, values url.Values, out any) error {
	return c.getJSON(ctx, c.baseV5, path, values, out)
}

func (c *Client) getJSON(ctx context.Context, baseURL string, path string, values url.Values, out any) error {
	if !c.Enabled() {
		return errors.New("AMAP_API_KEY is empty")
	}
	if strings.TrimSpace(baseURL) == "" {
		return errors.New("amap base URL is empty")
	}
	if values == nil {
		values = url.Values{}
	}
	values.Set("key", c.apiKey)
	values.Set("output", "JSON")

	reqURL := strings.TrimRight(baseURL, "/") + path + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("amap http failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode amap response: %w", err)
	}
	return nil
}
