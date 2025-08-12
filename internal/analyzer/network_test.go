package analyzer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	testLogger = slog.New(slog.NewTextHandler(&testWriter{}, nil))
)

type testWriter struct{}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestLoadWebPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("Success on first attempt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "Hello, client")
		}))
		defer server.Close()

		resp, err := loadWebPage(context.Background(), logger, server.URL)
		if err != nil {
			t.Fatalf("Expected no error, but got: %v", err)
		}
		if resp == nil {
			t.Fatal("Expected a response, but got nil")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Hello, client") {
			t.Errorf("Response body did not contain expected text. Got: %s", string(body))
		}
	})
}

func TestLinkAccessibilityChecker_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inaccessibleLinks := make(chan string, 1)

	linkAccessibilityChecker(context.Background(), testLogger, server.URL, inaccessibleLinks)

	select {
	case link := <-inaccessibleLinks:
		t.Errorf("Expected no inaccessible links, but got %s", link)
	default:
		// Test passes
	}
}

func TestLinkAccessibilityChecker_FailureAfterRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	inaccessibleLinks := make(chan string, 1)

	linkAccessibilityChecker(context.Background(), testLogger, server.URL, inaccessibleLinks)

	select {
	case link := <-inaccessibleLinks:
		if link != server.URL {
			t.Errorf("Expected link %s, but got %s", server.URL, link)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive an inaccessible link, but got none")
	}
}

func TestLinkAccessibilityChecker_SuccessAfterOneRetry(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inaccessibleLinks := make(chan string, 1)

	linkAccessibilityChecker(context.Background(), testLogger, server.URL, inaccessibleLinks)

	select {
	case link := <-inaccessibleLinks:
		t.Errorf("Expected no inaccessible links, but got %s", link)
	default:
		// Test passes
	}

	if atomic.LoadInt32(&requestCount) != 2 {
		t.Errorf("Expected 2 requests, but got %d", requestCount)
	}
}

func TestLinkAccessibilityChecker_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	inaccessibleLinks := make(chan string, 1)

	linkAccessibilityChecker(context.Background(), testLogger, server.URL, inaccessibleLinks)

	select {
	case link := <-inaccessibleLinks:
		if link != server.URL {
			t.Errorf("Expected link %s, but got %s", server.URL, link)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive an inaccessible link, but got none")
	}
}

func TestLinkAccessibilityChecker_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	inaccessibleLinks := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())

	time.AfterFunc(20*time.Millisecond, cancel)

	linkAccessibilityChecker(ctx, testLogger, server.URL, inaccessibleLinks)

	select {
	case link := <-inaccessibleLinks:
		if link != server.URL {
			t.Errorf("Expected link %s, but got %s", server.URL, link)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive an inaccessible link due to context cancellation, but got none")
	}
}

func TestValidateLinkAccessibility_AllLinksAccessible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	analysis := LinkAnalysis{
		InternalLinks: []string{server.URL + "/internal1", server.URL + "/internal2"},
		ExternalLinks: []string{server.URL + "/external1"},
	}

	failedLinks, err := validateLinkAccessibility(context.Background(), testLogger, analysis)
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	if len(failedLinks) != 0 {
		t.Errorf("Expected 0 failed links, but got %d: %v", len(failedLinks), failedLinks)
	}
}

func TestValidateLinkAccessibility_SomeLinksInaccessible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	analysis := LinkAnalysis{
		InternalLinks: []string{server.URL + "/ok", server.URL + "/fail"},
		ExternalLinks: []string{server.URL + "/another-ok"},
	}

	failedLinks, err := validateLinkAccessibility(context.Background(), testLogger, analysis)
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	if len(failedLinks) != 1 {
		t.Fatalf("Expected 1 failed link, but got %d", len(failedLinks))
	}

	expectedFailedLink := server.URL + "/fail"
	if failedLinks[0] != expectedFailedLink {
		t.Errorf("Expected failed link to be %s, but got %s", expectedFailedLink, failedLinks[0])
	}
}

func TestValidateLinkAccessibility_NoLinks(t *testing.T) {
	analysis := LinkAnalysis{
		InternalLinks: []string{},
		ExternalLinks: []string{},
	}

	failedLinks, err := validateLinkAccessibility(context.Background(), testLogger, analysis)
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	if len(failedLinks) != 0 {
		t.Errorf("Expected 0 failed links for empty input, but got %d", len(failedLinks))
	}
}

func TestWorker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	jobs := make(chan string, 2)
	inaccessibleLinks := make(chan string, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go linkAccessibilityCheckWorker(context.Background(), testLogger, &wg, jobs, inaccessibleLinks)

	jobs <- server.URL + "/good"
	jobs <- server.URL + "/bad"
	close(jobs)

	wg.Wait()
	close(inaccessibleLinks)

	var failedLinks []string
	for link := range inaccessibleLinks {
		failedLinks = append(failedLinks, link)
	}

	if len(failedLinks) != 1 {
		t.Fatalf("Expected 1 failed link, but got %d", len(failedLinks))
	}
	if failedLinks[0] != server.URL+"/bad" {
		t.Errorf("Expected failed link to be %s, but got %s", server.URL+"/bad", failedLinks[0])
	}
}
