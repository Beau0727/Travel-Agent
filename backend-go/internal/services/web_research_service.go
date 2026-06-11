package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/domain"
	"travel-agent-go/internal/geo"
	"travel-agent-go/internal/logging"
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
	Query    string
	Sources  []WebResearchSource
	Evidence domain.EvidenceReport
}

type WebResearchSource struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Host        string `json:"host,omitempty"`
	Snippet     string `json:"snippet"`
	PublishedAt string `json:"published_at,omitempty"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

type WebSearchProvider interface {
	Name() string
	Search(ctx context.Context, request WebSearchRequest) ([]WebSearchResult, error)
}

type WebSearchRequest struct {
	Query string
	Limit int
}

type WebSearchResult struct {
	Title       string
	URL         string
	Snippet     string
	PublishedAt string
	Provider    string
	Score       float64
}

type customEndpointSearchProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

type tavilySearchProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
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
			"query", logging.SafeText(query, 220),
			"query_chars", len([]rune(query)),
			"web_search_provider", s.cfg.WebSearchProvider,
			"web_search_endpoint_configured", strings.TrimSpace(s.cfg.WebSearchEndpoint) != "",
			"web_search_api_key_configured", strings.TrimSpace(s.cfg.WebSearchAPIKey) != "",
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
		"query", logging.SafeText(query, 220),
		"query_chars", len([]rune(query)),
		"requested_top_k", request.TopK,
		"max_pages", maxPages,
		"configured_max_pages", s.cfg.WebResearchMaxPages,
		"timeout_seconds", s.cfg.WebResearchTimeout,
		"web_search_provider", s.cfg.WebSearchProvider,
		"web_search_endpoint_configured", strings.TrimSpace(s.cfg.WebSearchEndpoint) != "",
		"web_search_api_key_configured", strings.TrimSpace(s.cfg.WebSearchAPIKey) != "",
	)

	searchLimit := maxPages * 3
	if searchLimit < maxPages {
		searchLimit = maxPages
	}
	searchResults, err := s.search(ctx, query, searchLimit)
	if err != nil {
		logging.Warn(ctx, "web research search failed",
			"query", logging.SafeText(query, 220),
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return WebResearchResult{}, err
	}
	logging.Info(ctx, "web research search completed",
		"query", logging.SafeText(query, 220),
		"candidate_results", len(searchResults),
	)
	searchResults = prioritizeSearchResults(searchResults)
	sources := make([]WebResearchSource, 0, len(searchResults))
	for _, searchResult := range searchResults {
		source, err := s.fetchSource(ctx, searchResult.URL)
		if err != nil || source.Snippet == "" {
			if err != nil {
				logging.Warn(ctx, "web research source skipped",
					"url", searchResult.URL,
					"error", err,
				)
			} else {
				logging.Warn(ctx, "web research source skipped empty snippet",
					"url", searchResult.URL,
				)
			}
			source = sourceFromSearchResult(searchResult)
			if source.Snippet == "" {
				continue
			}
		} else {
			source = mergeFetchedSource(searchResult, source)
		}
		if !webResearchSourceMatchesDestination(request.Destination, source) {
			logging.Warn(ctx, "web research source rejected by destination filter",
				"destination", request.Destination,
				"url", source.URL,
				"title", source.Title,
			)
			continue
		}
		sources = append(sources, source)
		logging.Info(ctx, "web research source accepted",
			"url", source.URL,
			"title", source.Title,
			"provider", source.Provider,
			"snippet_chars", len([]rune(source.Snippet)),
			"accepted", len(sources),
		)
		if len(sources) >= maxPages {
			break
		}
	}
	logging.Info(ctx, "web research completed",
		"query", logging.SafeText(query, 220),
		"sources", len(sources),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	evidence := buildEvidenceReport(request.Destination, query, sources, time.Now())
	logging.Info(ctx, "web research evidence built",
		"query", logging.SafeText(query, 220),
		"sources", len(evidence.Sources),
		"claims", len(evidence.Claims),
		"warnings", len(evidence.Warnings),
	)
	return WebResearchResult{Query: query, Sources: sources, Evidence: evidence}, nil
}

func (s *WebResearchService) search(ctx context.Context, query string, limit int) ([]WebSearchResult, error) {
	provider, err := selectWebSearchProvider(s.cfg, s.client)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		logging.Warn(ctx, "web search provider unavailable",
			"provider", s.cfg.WebSearchProvider,
			"endpoint_configured", strings.TrimSpace(s.cfg.WebSearchEndpoint) != "",
			"api_key_configured", strings.TrimSpace(s.cfg.WebSearchAPIKey) != "",
		)
		return nil, nil
	}

	start := time.Now()
	logging.Info(ctx, "web search request started",
		"provider", provider.Name(),
		"query", logging.SafeText(query, 220),
		"query_chars", len([]rune(query)),
		"limit", limit,
	)
	results, err := provider.Search(ctx, WebSearchRequest{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		logging.Warn(ctx, "web search request failed",
			"provider", provider.Name(),
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, err
	}
	results = normalizeSearchResults(results, provider.Name(), limit)
	logging.Info(ctx, "web search request completed",
		"provider", provider.Name(),
		"results", len(results),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return results, nil
}

func selectWebSearchProvider(cfg config.Config, client *http.Client) (WebSearchProvider, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.WebSearchProvider))
	endpoint := strings.TrimSpace(cfg.WebSearchEndpoint)
	apiKey := strings.TrimSpace(cfg.WebSearchAPIKey)

	if provider == "" {
		switch {
		case endpoint != "":
			provider = "custom"
		case apiKey != "":
			provider = "tavily"
		default:
			return nil, nil
		}
	}

	switch provider {
	case "tavily":
		if apiKey == "" {
			return nil, errors.New("WEB_SEARCH_API_KEY is required when WEB_SEARCH_PROVIDER=tavily")
		}
		if endpoint == "" {
			endpoint = "https://api.tavily.com/search"
		}
		return tavilySearchProvider{endpoint: endpoint, apiKey: apiKey, client: client}, nil
	case "custom", "endpoint", "proxy":
		if endpoint == "" {
			return nil, errors.New("WEB_SEARCH_ENDPOINT is required when WEB_SEARCH_PROVIDER=custom")
		}
		return customEndpointSearchProvider{endpoint: endpoint, apiKey: apiKey, client: client}, nil
	default:
		return nil, fmt.Errorf("unsupported WEB_SEARCH_PROVIDER %q", cfg.WebSearchProvider)
	}
}

func (p customEndpointSearchProvider) Name() string {
	return "custom"
}

func (p customEndpointSearchProvider) Search(ctx context.Context, request WebSearchRequest) ([]WebSearchResult, error) {
	searchURL, err := url.Parse(p.endpoint)
	if err != nil {
		return nil, err
	}
	limit := normalizedLimit(request.Limit)
	values := searchURL.Query()
	values.Set("q", request.Query)
	values.Set("query", request.Query)
	values.Set("limit", itoa(limit))
	values.Set("max_results", itoa(limit))
	searchURL.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("X-API-Key", p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 800))
		return nil, errors.New(strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseSearchResults(body, p.Name(), limit), nil
}

func (p tavilySearchProvider) Name() string {
	return "tavily"
}

func (p tavilySearchProvider) Search(ctx context.Context, request WebSearchRequest) ([]WebSearchResult, error) {
	limit := normalizedLimit(request.Limit)
	payload := map[string]any{
		"query":               request.Query,
		"max_results":         limit,
		"search_depth":        "advanced",
		"include_answer":      false,
		"include_images":      false,
		"include_raw_content": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1200))
		return nil, fmt.Errorf("tavily search failed: %s", strings.TrimSpace(string(body)))
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseSearchResults(responseBody, p.Name(), limit), nil
}

func (s *WebResearchService) fetchSource(ctx context.Context, pageURL string) (WebResearchSource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return WebResearchSource{}, err
	}
	req.Header.Set("User-Agent", "travel-agent-go/0.1 travel research bot")

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
		Title:       title,
		URL:         pageURL,
		Host:        hostFromURL(pageURL),
		Snippet:     snippet,
		RetrievedAt: time.Now().Format(time.RFC3339),
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
	return destination + " 旅游攻略 官网 官方 地图 门票 开放时间 预约 景点 美食 行程"
}

func sourceFromSearchResult(result WebSearchResult) WebResearchSource {
	snippet := strings.TrimSpace(result.Snippet)
	if snippet == "" {
		snippet = strings.TrimSpace(result.Title)
	}
	return WebResearchSource{
		Title:       strings.TrimSpace(defaultString(result.Title, hostFromURL(result.URL))),
		URL:         strings.TrimSpace(result.URL),
		Host:        hostFromURL(result.URL),
		Snippet:     trimRunes(snippet, 900),
		PublishedAt: strings.TrimSpace(result.PublishedAt),
		RetrievedAt: time.Now().Format(time.RFC3339),
		Provider:    strings.TrimSpace(result.Provider),
	}
}

func mergeFetchedSource(result WebSearchResult, source WebResearchSource) WebResearchSource {
	if strings.TrimSpace(source.Title) == "" {
		source.Title = strings.TrimSpace(result.Title)
	}
	if strings.TrimSpace(source.Snippet) == "" {
		source.Snippet = strings.TrimSpace(result.Snippet)
	}
	if strings.TrimSpace(source.PublishedAt) == "" {
		source.PublishedAt = strings.TrimSpace(result.PublishedAt)
	}
	if strings.TrimSpace(source.Provider) == "" {
		source.Provider = strings.TrimSpace(result.Provider)
	}
	if strings.TrimSpace(source.Host) == "" {
		source.Host = hostFromURL(source.URL)
	}
	return source
}

func webResearchSourceMatchesDestination(destination string, source WebResearchSource) bool {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return true
	}
	text := strings.Join([]string{source.Title, source.URL, source.Host, source.Snippet}, " ")
	if !geo.TextMatchesDestination(destination, text) {
		return false
	}
	titleText := strings.Join([]string{source.Title, source.URL, source.Host}, " ")
	if geo.HasConflictingCityMention(destination, titleText) && !geo.TextMatchesDestination(destination, source.Title) {
		return false
	}
	return true
}

func prioritizeSearchResults(results []WebSearchResult) []WebSearchResult {
	type rankedResult struct {
		result   WebSearchResult
		priority int
		score    float64
		index    int
	}
	ranked := make([]rankedResult, 0, len(results))
	for index, item := range results {
		host := hostFromURL(item.URL)
		sourceType, _, _, _ := classifySource(item.Title, item.URL, host)
		ranked = append(ranked, rankedResult{
			result:   item,
			priority: sourcePriority(sourceType),
			score:    item.Score,
			index:    index,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].priority != ranked[j].priority {
			return ranked[i].priority < ranked[j].priority
		}
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].index < ranked[j].index
	})
	prioritized := make([]WebSearchResult, 0, len(ranked))
	for _, item := range ranked {
		prioritized = append(prioritized, item.result)
	}
	return prioritized
}

func normalizeSearchResults(results []WebSearchResult, provider string, limit int) []WebSearchResult {
	limit = normalizedLimit(limit)
	seen := map[string]bool{}
	normalized := make([]WebSearchResult, 0, minInt(len(results), limit))
	for _, item := range results {
		item.URL = normalizeHTTPURL(item.URL)
		if item.URL == "" || seen[item.URL] || len(normalized) >= limit {
			continue
		}
		seen[item.URL] = true
		item.Title = strings.TrimSpace(item.Title)
		item.Snippet = strings.TrimSpace(item.Snippet)
		item.PublishedAt = strings.TrimSpace(item.PublishedAt)
		if strings.TrimSpace(item.Provider) == "" {
			item.Provider = provider
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func parseSearchResults(body []byte, provider string, limit int) []WebSearchResult {
	limit = normalizedLimit(limit)
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	seen := map[string]bool{}
	results := []WebSearchResult{}
	var walk func(any)
	walk = func(value any) {
		if len(results) >= limit {
			return
		}
		switch typed := value.(type) {
		case map[string]any:
			if result, ok := searchResultFromMap(typed, provider); ok {
				addSearchResult(result, seen, &results, limit)
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

func searchResultFromMap(item map[string]any, provider string) (WebSearchResult, bool) {
	rawURL := firstStringValue(item, "url", "link", "href")
	if rawURL == "" {
		return WebSearchResult{}, false
	}
	return WebSearchResult{
		Title:       firstStringValue(item, "title", "name"),
		URL:         rawURL,
		Snippet:     firstStringValue(item, "content", "snippet", "description", "summary"),
		PublishedAt: firstStringValue(item, "published_at", "publishedAt", "published_date", "date"),
		Provider:    provider,
		Score:       firstFloatValue(item, "score"),
	}, true
}

func firstStringValue(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstFloatValue(item map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := item[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case json.Number:
			parsed, _ := value.Float64()
			return parsed
		}
	}
	return 0
}

func addSearchResult(result WebSearchResult, seen map[string]bool, results *[]WebSearchResult, limit int) {
	normalized := normalizeHTTPURL(result.URL)
	if normalized == "" || seen[normalized] || len(*results) >= limit {
		return
	}
	result.URL = normalized
	seen[normalized] = true
	*results = append(*results, result)
}

func normalizeHTTPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.String()
}

func normalizedLimit(limit int) int {
	if limit <= 0 {
		return 3
	}
	if limit > 20 {
		return 20
	}
	return limit
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
