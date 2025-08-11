package analyzer

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	client = &http.Client{Timeout: 1 * time.Second}
	os.Exit(m.Run())
}

func TestLinkAccessibilityChecker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	t.Run("AccessibleLink", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		var wg sync.WaitGroup
		inaccessibleLinks := make(chan string, 1)

		wg.Add(1)
		go linkAccessibilityChecker(ctx, logger, server.URL, &wg, inaccessibleLinks)
		wg.Wait()
		close(inaccessibleLinks)

		if len(inaccessibleLinks) != 0 {
			t.Errorf("Expected 0 inaccessible links, but got %d", len(inaccessibleLinks))
		}
	})

	t.Run("InaccessibleLinkAfterRetries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		var wg sync.WaitGroup
		inaccessibleLinks := make(chan string, 1)

		wg.Add(1)
		go linkAccessibilityChecker(ctx, logger, server.URL, &wg, inaccessibleLinks)
		wg.Wait()
		close(inaccessibleLinks)

		if len(inaccessibleLinks) != 1 {
			t.Errorf("Expected 1 inaccessible link, but got %d", len(inaccessibleLinks))
		}
		if link := <-inaccessibleLinks; link != server.URL {
			t.Errorf("Expected link %s to be inaccessible, but it was not reported", server.URL)
		}
	})

	t.Run("AccessibleAfterOneRetry", func(t *testing.T) {
		attempt := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if attempt == 0 {
				attempt++
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		var wg sync.WaitGroup
		inaccessibleLinks := make(chan string, 1)

		wg.Add(1)
		go linkAccessibilityChecker(ctx, logger, server.URL, &wg, inaccessibleLinks)
		wg.Wait()
		close(inaccessibleLinks)

		if len(inaccessibleLinks) != 0 {
			t.Errorf("Expected 0 inaccessible links, but got %d", len(inaccessibleLinks))
		}
	})

	t.Run("InvalidURL", func(t *testing.T) {
		var wg sync.WaitGroup
		inaccessibleLinks := make(chan string, 1)
		invalidURL := "ht tp://invalid-url"

		wg.Add(1)
		go linkAccessibilityChecker(ctx, logger, invalidURL, &wg, inaccessibleLinks)
		wg.Wait()
		close(inaccessibleLinks)

		if len(inaccessibleLinks) != 1 {
			t.Errorf("Expected 1 inaccessible link for invalid URL, but got %d", len(inaccessibleLinks))
		}
		if link := <-inaccessibleLinks; link != invalidURL {
			t.Errorf("Expected link %s to be reported as inaccessible", invalidURL)
		}
	})
}

func TestValidateLinkAccessibility(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	t.Run("MixedAccessibility", func(t *testing.T) {
		analysis := LinkAnalysis{
			InternalLinks: []string{okServer.URL, failServer.URL},
			ExternalLinks: []string{okServer.URL},
		}

		failedLinks, err := validateLinkAccessibility(ctx, logger, analysis)
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		if len(failedLinks) != 1 {
			t.Errorf("Expected 1 failed link, but got %d", len(failedLinks))
		}
		if failedLinks[0] != failServer.URL {
			t.Errorf("Expected %s to be in failed links, but it was not", failServer.URL)
		}
	})

	t.Run("AllLinksAccessible", func(t *testing.T) {
		analysis := LinkAnalysis{
			InternalLinks: []string{okServer.URL},
			ExternalLinks: []string{okServer.URL},
		}

		failedLinks, err := validateLinkAccessibility(ctx, logger, analysis)
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		if len(failedLinks) != 0 {
			t.Errorf("Expected 0 failed links, but got %d", len(failedLinks))
		}
	})

	t.Run("NoLinksToCheck", func(t *testing.T) {
		analysis := LinkAnalysis{
			InternalLinks: []string{},
			ExternalLinks: []string{},
		}

		failedLinks, err := validateLinkAccessibility(ctx, logger, analysis)
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		if len(failedLinks) != 0 {
			t.Errorf("Expected 0 failed links, but got %d", len(failedLinks))
		}
	})
}
