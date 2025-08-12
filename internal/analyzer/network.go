package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"
)

const (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
	numWorkers     = 10
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

		fmt.Println(data)

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

	if data != nil && err == nil {
		issue := fmt.Errorf("access denied to the page %v", data)
		logger.ErrorContext(
			ctx,
			"Failed to fetch page after all attempts",
			slog.Int("max_retries", maxRetries),
			slog.Any("last_error", issue),
		)
		return nil, issue
	} else {
		logger.ErrorContext(
			ctx,
			"Failed to fetch page after all attempts",
			slog.Int("max_retries", maxRetries),
			slog.Any("last_error", err),
		)
		return nil, err
	}
}

func linkAccessibilityChecker(ctx context.Context, logger *slog.Logger, url string, inaccessibleLinks chan<- string) {
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

func linkAccessibilityCheckWorker(ctx context.Context, logger *slog.Logger, wg *sync.WaitGroup, jobs <-chan string, inaccessibleLinks chan<- string) {
	defer wg.Done()
	for url := range jobs {
		linkAccessibilityChecker(ctx, logger, url, inaccessibleLinks)
	}
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

	jobs := make(chan string, totalLinks)
	inaccessibleLinks := make(chan string, totalLinks)

	var wg sync.WaitGroup

	// Prevent creating unnecessary additional workers
	if totalLinks < numWorkers {
		for w := 1; w <= totalLinks; w++ {
			wg.Add(1)
			go linkAccessibilityCheckWorker(ctx, logger, &wg, jobs, inaccessibleLinks)
		}
	} else {
		for w := 1; w <= numWorkers; w++ {
			wg.Add(1)
			go linkAccessibilityCheckWorker(ctx, logger, &wg, jobs, inaccessibleLinks)
		}
	}

	for _, link := range pageLinks {
		jobs <- link
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(inaccessibleLinks)
	}()

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
