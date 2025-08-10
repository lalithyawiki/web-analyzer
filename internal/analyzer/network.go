package analyzer

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"
)

const (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
)

var client = &http.Client{
	Timeout: 10 * time.Second,
}

func loadWebPage(ctx context.Context, logger *slog.Logger, pageURL string) (*http.Response, error) {
	logger = logger.With(slog.String("analyzing_page_link", pageURL))

	logger.DebugContext(ctx, "Starting to load web page")

	var err error
	var data *http.Response

	for i := 0; i < maxRetries; i++ {
		attempt := i + 1
		logger.DebugContext(ctx, "Attempting to fetch page", slog.Int("attempt", attempt))

		data, err = http.Get(pageURL)

		if err == nil && data.StatusCode >= 200 && data.StatusCode < 300 {
			logger.InfoContext(
				ctx,
				"Successfully fetched page",
				slog.Int("status_code", data.StatusCode),
				slog.Int("attempt", attempt),
			)
			return data, nil
		}

		if data != nil {
			data.Body.Close()
		}

		if i < maxRetries-1 {
			backoffDuration := initialBackoff * time.Duration(math.Pow(2, float64(i)))
			var statusCode int
			if data != nil {
				statusCode = data.StatusCode
			}

			logger.WarnContext(
				ctx,
				"Fetch attempt failed, retrying...",
				slog.Int("attempt", attempt),
				slog.Any("error", err),
				slog.Int("status_code", statusCode),
				slog.Duration("backoff_duration", backoffDuration),
			)
			time.Sleep(backoffDuration)
			continue
		}
	}

	logger.ErrorContext(
		ctx,
		"Failed to fetch page after all attempts",
		slog.Int("max_retries", maxRetries),
		slog.Any("last_error", err),
	)
	return nil, err
}

func linkAccessibilityChecker(ctx context.Context, logger *slog.Logger, url string, wg *sync.WaitGroup, inaccessibleLinks chan<- string) {
	defer wg.Done()

	logger = logger.With(slog.String("url", url))
	logger.DebugContext(ctx, "Starting link check")

	backoff := initialBackoff
	for i := 0; i < maxRetries; i++ {
		attempt := i + 1
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			logger.ErrorContext(ctx, "Could not create HTTP request", slog.Any("error", err))
			inaccessibleLinks <- url
			return
		}

		resp, err := client.Do(req)

		if err != nil {
			logger.WarnContext(ctx, "Connection error on attempt, retrying...",
				slog.Int("attempt", attempt),
				slog.Any("error", err),
				slog.Duration("backoff_duration", backoff),
			)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logger.InfoContext(ctx, "Link is accessible", slog.Int("status_code", resp.StatusCode))
			resp.Body.Close()
			return
		}

		logger.WarnContext(ctx, "Received non-success status, retrying...",
			slog.Int("attempt", attempt),
			slog.Int("status_code", resp.StatusCode),
			slog.String("status_text", resp.Status),
			slog.Duration("backoff_duration", backoff),
		)
		resp.Body.Close()

		time.Sleep(backoff)
		backoff *= 2
	}

	logger.ErrorContext(ctx, "Link is inaccessible after all retries", slog.Int("max_retries", maxRetries))
	inaccessibleLinks <- url
}

func validateLinkAccessibility(ctx context.Context, logger *slog.Logger, analysis LinkAnalysis) ([]string, error) {
	logger.DebugContext(ctx, "Setting up link check process")

	pageLinks := append(analysis.InternalLinks, analysis.ExternalLinks...)
	if len(pageLinks) == 0 {
		logger.InfoContext(ctx, "No links to check, skipping process.")
		return nil, nil
	}

	totalLinks := len(pageLinks)
	logger.InfoContext(ctx, "Starting to check links", slog.Int("total_links", totalLinks))

	var wg sync.WaitGroup
	inaccessibleLinks := make(chan string, totalLinks)

	for _, link := range pageLinks {
		wg.Add(1)
		go linkAccessibilityChecker(ctx, logger, link, &wg, inaccessibleLinks)
	}

	wg.Wait()
	close(inaccessibleLinks)

	var failedLinks []string
	for link := range inaccessibleLinks {
		failedLinks = append(failedLinks, link)
	}

	logger.InfoContext(ctx, "Finished checking all links",
		slog.Int("total_links_checked", totalLinks),
		slog.Int("inaccessible_links_found", len(failedLinks)),
	)

	return failedLinks, nil
}
