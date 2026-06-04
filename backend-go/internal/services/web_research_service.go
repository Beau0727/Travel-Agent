package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"zhilv-yuntu-go/internal/config"
	"zhilv-yuntu-go/internal/logging"
)

type WebResearchService struct {
	cfg    config.Config
	client *http.Client
}

type WebResearchRequest struct {
	Destination string
	Query       string
	TopK        int
}

type WebResearchResult struct {
	Query   string
	Sources []WebResearchSource
}

type WebResearchSource struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func NewWebResearchService(cfg config.Config) *WebResearchService {
	timeout := cfg.WebResearchTimeout
	if timeout <= 0 {
		timeout = 20
	}
	return &WebResearchService{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (s *WebResearchService) Research(ctx context.Context, request WebResearchRequest) (WebResearchResult, error) {
	start := time.Now()
	query := buildWebResearchQuery(request.Destination, request.Query)
	if strings.TrimSpace(query) == "" {
		return WebResearchResult{}, errors.New("web research query is empty")
	}
	if !s.cfg.EnableWebResearch {
		logging.Info(ctx, "web research disabled",
			"destination", request.Destination,
			"query", query,
		)
		return WebResearchResult{Query: query}, nil
	}

	maxPages := request.TopK
	if maxPages <= 0 {
		maxPages = s.cfg.WebResearchMaxPages
	}
	if maxPages <= 0 {
		maxPages = 3
	}
	logging.Info(ctx, "web research started",
		"destination", request.Destination,
		"query", query,
		"max_pages", maxPages,
	)

	urls, err := s.search(ctx, query, maxPages)
	if err != nil {
		logging.Warn(ctx, "web research search failed",
			"query", query,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return WebResearchResult{}, err
	}
	logging.Info(ctx, "web research search completed",
		"query", query,
		"candidate_urls", len(urls),
	)
	sources := make([]WebResearchSource, 0, len(urls))
	for _, pageURL := range urls {
		source, err := s.fetchSource(ctx, pageURL)
		if err != nil || source.Snippet == "" {
			if err != nil {
				logging.Warn(ctx, "web research source skipped",
					"url", pageURL,
					"error", err,
				)
			} else {
				logging.Warn(ctx, "web research source skipped empty snippet",
					"url", pageURL,
				)
			}
			continue
		}
		sources = append(sources, source)
		logging.Info(ctx, "web research source accepted",
			"url", pageURL,
			"title", source.Title,
			"snippet_chars", len([]rune(source.Snippet)),
			"accepted", len(sources),
		)
		if len(sources) >= maxPages {
			break
		}
	}
	logging.Info(ctx, "web research completed",
		"query", query,
		"sources", len(sources),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return WebResearchResult{Query: query, Sources: sources}, nil
}

func (s *WebResearchService) search(ctx context.Context, query string, limit int) ([]string, error) {
	endpoint := strings.TrimSpace(s.cfg.WebSearchEndpoint)
	if endpoint == "" {
		logging.Warn(ctx, "web search endpoint empty")
		return nil, nil
	}

	searchURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	values := searchURL.Query()
	values.Set("q", query)
	values.Set("query", query)
	values.Set("limit", itoa(limit))
	searchURL.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, err
	}
	if s.cfg.WebSearchAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.WebSearchAPIKey)
		req.Header.Set("X-API-Key", s.cfg.WebSearchAPIKey)
	}

	start := time.Now()
	logging.Info(ctx, "web search request started",
		"endpoint", endpoint,
		"query", query,
		"limit", limit,
	)
	resp, err := s.client.Do(req)
	if err != nil {
		logging.Warn(ctx, "web search request failed",
			"endpoint", endpoint,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 800))
		logging.Warn(ctx, "web search response failed",
			"endpoint", endpoint,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", strings.TrimSpace(string(body)),
		)
		return nil, errors.New(strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	urls := parseSearchURLs(body, limit)
	logging.Info(ctx, "web search request completed",
		"endpoint", endpoint,
		"status", resp.StatusCode,
		"urls", len(urls),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return urls, nil
}

func (s *WebResearchService) fetchSource(ctx context.Context, pageURL string) (WebResearchSource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return WebResearchSource{}, err
	}
	req.Header.Set("User-Agent", "zhilv-yuntu-go/0.1 travel research bot")

	start := time.Now()
	logging.Info(ctx, "web page fetch started", "url", pageURL)
	resp, err := s.client.Do(req)
	if err != nil {
		logging.Warn(ctx, "web page fetch failed",
			"url", pageURL,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return WebResearchSource{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Warn(ctx, "web page response failed",
			"url", pageURL,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return WebResearchSource{}, errors.New(resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "text/html") {
		logging.Warn(ctx, "web page unsupported content type",
			"url", pageURL,
			"content_type", contentType,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return WebResearchSource{}, errors.New("unsupported content type: " + contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return WebResearchSource{}, err
	}
	title, snippet := extractHTMLText(string(body))
	logging.Info(ctx, "web page fetch completed",
		"url", pageURL,
		"status", resp.StatusCode,
		"title", title,
		"snippet_chars", len([]rune(snippet)),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return WebResearchSource{
		Title:   title,
		URL:     pageURL,
		Snippet: snippet,
	}, nil
}

func buildWebResearchQuery(destination, query string) string {
	query = strings.TrimSpace(query)
	if query != "" {
		return query
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return ""
	}
	return destination + " 旅游攻略 景点 美食 行程"
}

func parseSearchURLs(body []byte, limit int) []string {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	seen := map[string]bool{}
	results := []string{}
	var walk func(any)
	walk = func(value any) {
		if len(results) >= limit {
			return
		}
		switch typed := value.(type) {
		case map[string]any:
			for _, key := range []string{"url", "link", "href"} {
				if raw, ok := typed[key].(string); ok {
					addURL(raw, seen, &results, limit)
				}
			}
			for _, item := range typed {
				walk(item)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(payload)
	return results
}

func addURL(raw string, seen map[string]bool, results *[]string, limit int) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return
	}
	normalized := parsed.String()
	if seen[normalized] || len(*results) >= limit {
		return
	}
	seen[normalized] = true
	*results = append(*results, normalized)
}

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	titleRe       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	tagRe         = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRe       = regexp.MustCompile(`\s+`)
)

func extractHTMLText(html string) (string, string) {
	title := ""
	if match := titleRe.FindStringSubmatch(html); len(match) > 1 {
		title = cleanHTMLText(match[1])
	}
	text := scriptStyleRe.ReplaceAllString(html, " ")
	text = tagRe.ReplaceAllString(text, " ")
	text = cleanHTMLText(text)
	if len([]rune(text)) > 900 {
		runes := []rune(text)
		text = string(runes[:900])
	}
	return title, text
}

func cleanHTMLText(value string) string {
	replacements := map[string]string{
		"&nbsp;": " ",
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": `"`,
		"&#39;":  "'",
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return strings.TrimSpace(spaceRe.ReplaceAllString(value, " "))
}
