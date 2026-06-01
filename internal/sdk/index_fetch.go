package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/inherelab/eget/internal/client"
)

func (s Service) fetchIndex(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	body, err := s.fetchIndexBytes(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (s Service) fetchIndexBytes(ctx context.Context, rawURL string) ([]byte, error) {
	clientOpts := s.effectiveClientOptions()
	httpClient, err := newDownloadHTTPClient(clientOpts)
	if err != nil {
		return nil, err
	}
	body, pagination, err := s.fetchIndexPage(ctx, httpClient, clientOpts, rawURL)
	if err != nil {
		return nil, err
	}
	if pagination.NextPage == 0 {
		return body, nil
	}

	var merged []json.RawMessage
	if err := json.Unmarshal(body, &merged); err != nil {
		return nil, fmt.Errorf("paginated sdk index response must be a json array: %w", err)
	}
	seenPages := map[int]bool{}
	for pagination.NextPage > 0 {
		nextPage := pagination.NextPage
		if seenPages[nextPage] {
			return nil, fmt.Errorf("sdk index pagination loop detected at page %d", nextPage)
		}
		seenPages[nextPage] = true
		nextURL, err := indexPageURL(rawURL, nextPage)
		if err != nil {
			return nil, err
		}
		body, pagination, err = s.fetchIndexPage(ctx, httpClient, clientOpts, nextURL)
		if err != nil {
			return nil, err
		}
		var pageItems []json.RawMessage
		if err := json.Unmarshal(body, &pageItems); err != nil {
			return nil, fmt.Errorf("paginated sdk index response must be a json array: %w", err)
		}
		merged = append(merged, pageItems...)
	}
	return json.Marshal(merged)
}

func (s Service) fetchIndexPage(ctx context.Context, httpClient *http.Client, clientOpts client.Options, rawURL string) ([]byte, indexPagination, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, indexPagination{}, err
	}
	userAgent := clientOpts.UserAgent
	if userAgent == "" {
		userAgent = client.DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, indexPagination{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, indexPagination{}, fmt.Errorf("sdk index request failed: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, indexPagination{}, err
	}
	pagination, err := parseIndexPagination(resp.Header.Get("X-Pagination"))
	if err != nil {
		return nil, indexPagination{}, err
	}
	return body, pagination, nil
}

type indexPagination struct {
	NextPage int `json:"next_page"`
}

func parseIndexPagination(header string) (indexPagination, error) {
	if strings.TrimSpace(header) == "" {
		return indexPagination{}, nil
	}
	var pagination indexPagination
	if err := json.Unmarshal([]byte(header), &pagination); err != nil {
		return indexPagination{}, fmt.Errorf("parse sdk index pagination header: %w", err)
	}
	return pagination, nil
}

func indexPageURL(rawURL string, page int) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("page", fmt.Sprint(page))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
