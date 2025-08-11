package analyzer

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFindHTMLVersion(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	testCases := []struct {
		name        string
		htmlContent string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "HTML5",
			htmlContent: `<!DOCTYPE html><html><head></head><body></body></html>`,
			wantVersion: "HTML5",
			wantErr:     false,
		},
		{
			name:        "XHTML 1.0 Transitional",
			htmlContent: `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd"><html><head></head><body></body></html>`,
			wantVersion: "XHTML 1.0",
			wantErr:     false,
		},
		{
			name:        "HTML 4.01 Strict",
			htmlContent: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd"><html><head></head><body></body></html>`,
			wantVersion: "HTML 4.01",
			wantErr:     false,
		},
		{
			name:        "Unknown Pre-HTML5",
			htmlContent: `<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN"><html><head></head><body></body></html>`,
			wantVersion: "Unknown (Pre-HTML5)",
			wantErr:     false,
		},
		{
			name:        "No Doctype",
			htmlContent: `<html><head></head><body><h1>Hello</h1></body></html>`,
			wantVersion: "Unknown or No Doctype",
			wantErr:     false,
		},
		{
			name:        "Empty Document",
			htmlContent: ``,
			wantVersion: "Unknown or No Doctype",
			wantErr:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.htmlContent))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			version, err := findHTMLVersion(ctx, logger, doc)

			if (err != nil) != tc.wantErr {
				t.Errorf("findHTMLVersion() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if version != tc.wantVersion {
				t.Errorf("findHTMLVersion() got = %v, want %v", version, tc.wantVersion)
			}
		})
	}
}

func TestCountHeadings(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	testCases := []struct {
		name        string
		htmlContent string
		wantCounts  map[string]int
		wantErr     bool
	}{
		{
			name:        "All Heading Levels",
			htmlContent: `<html><body><h1>H1</h1><h2>H2</h2><h3>H3</h3><h4>H4</h4><h5>H5</h5><h6>H6</h6><h2>Another H2</h2></body></html>`,
			wantCounts:  map[string]int{"h1": 1, "h2": 2, "h3": 1, "h4": 1, "h5": 1, "h6": 1},
			wantErr:     false,
		},
		{
			name:        "No Headings",
			htmlContent: `<html><body><p>Just a paragraph.</p></body></html>`,
			wantCounts:  map[string]int{},
			wantErr:     false,
		},
		{
			name:        "Only Some Heading Levels",
			htmlContent: `<html><body><h1>H1</h1><h3>H3</h3><h3>H3</h3></body></html>`,
			wantCounts:  map[string]int{"h1": 1, "h3": 2},
			wantErr:     false,
		},
		{
			name:        "Empty Document",
			htmlContent: ``,
			wantCounts:  map[string]int{},
			wantErr:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.htmlContent))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			counts, err := countHeadings(ctx, logger, doc)

			if (err != nil) != tc.wantErr {
				t.Errorf("countHeadings() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !reflect.DeepEqual(counts, tc.wantCounts) {
				t.Errorf("countHeadings() got = %v, want %v", counts, tc.wantCounts)
			}
		})
	}
}

func TestExtractLinks(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	baseURL, _ := url.Parse("https://example.com/path/")

	testCases := []struct {
		name        string
		htmlContent string
		wantResult  LinkAnalysis
		wantErr     bool
	}{
		{
			name: "Mix of Internal and External Links",
			htmlContent: `
                <a href="/about">About</a>
                <a href="https://google.com">Google</a>
                <a href="contact.html">Contact</a>
                <a href="https://sub.example.com/page">Subdomain (External)</a>
            `,
			wantResult: LinkAnalysis{
				InternalLinks: []string{"https://example.com/about", "https://example.com/path/contact.html"},
				ExternalLinks: []string{"https://google.com/", "https://sub.example.com/page"},
			},
			wantErr: false,
		},
		{
			name: "Skipped Links",
			htmlContent: `
                <a href="#">Fragment</a>
                <a href="mailto:test@example.com">Mail</a>
                <a href="tel:+123456789">Tel</a>
                <a href="  ">Whitespace</a>
            `,
			wantResult: LinkAnalysis{
				InternalLinks: []string{},
				ExternalLinks: []string{},
			},
			wantErr: false,
		},
		{
			name: "Absolute Internal Links",
			htmlContent: `
                <a href="https://example.com/another-page">Another Page</a>
            `,
			wantResult: LinkAnalysis{
				InternalLinks: []string{"https://example.com/another-page"},
				ExternalLinks: []string{},
			},
			wantErr: false,
		},
		{
			name: "Invalid Href",
			htmlContent: `
                <a href="http://a b.com/">Invalid</a>
                <a href="/good-link">Good</a>
            `,
			wantResult: LinkAnalysis{
				InternalLinks: []string{"https://example.com/good-link"},
				ExternalLinks: []string{},
			},
			wantErr: true,
		},
		{
			name:        "Empty Document",
			htmlContent: ``,
			wantResult: LinkAnalysis{
				InternalLinks: []string{},
				ExternalLinks: []string{},
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.htmlContent))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			result, err := extractLinks(ctx, logger, doc, baseURL)

			if (err != nil) != tc.wantErr {
				t.Errorf("extractLinks() error = %v, wantErr %v", err, tc.wantErr)
			}

			if len(result.InternalLinks) != len(tc.wantResult.InternalLinks) ||
				len(result.ExternalLinks) != len(tc.wantResult.ExternalLinks) {
				t.Errorf("extractLinks() result length mismatch. Got internal %d, external %d. Want internal %d, external %d",
					len(result.InternalLinks), len(result.ExternalLinks), len(tc.wantResult.InternalLinks), len(tc.wantResult.ExternalLinks))
			}
		})
	}
}

func TestDetectLoginForm(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	testCases := []struct {
		name        string
		htmlContent string
		want        bool
		wantErr     bool
	}{
		{
			name: "Standard Login Form",
			htmlContent: `
                <form>
                    <input type="email" name="username">
                    <input type="password" name="password">
                    <button type="submit">Log In</button>
                </form>
            `,
			want:    true,
			wantErr: false,
		},
		{
			name: "Sign In Button",
			htmlContent: `
                <form>
                    <input type="text" id="user-id">
                    <input type="password" id="pass">
                    <input type="submit" value="Sign In">
                </form>
            `,
			want:    true,
			wantErr: false,
		},
		{
			name: "No Login Button but has User Identifier",
			htmlContent: `
                <form>
                    <input type="text" placeholder="Enter your email">
                    <input type="password" name="pword">
                    <button type="submit">Go</button>
                </form>
            `,
			want:    true,
			wantErr: false,
		},
		{
			name: "No Password Field",
			htmlContent: `
                <form>
                    <input type="email" name="email">
                    <button type="submit">Subscribe</button>
                </form>
            `,
			want:    false,
			wantErr: false,
		},
		{
			name: "Search Form - Not a Login Form",
			htmlContent: `
                <form action="/search">
                    <input type="search" name="q">
                    <button type="submit">Search</button>
                </form>
            `,
			want:    false,
			wantErr: false,
		},
		{
			name: "Fallback JS-based Login - No Form Tag",
			htmlContent: `
                <div>
                    <input type="email" id="email-field">
                    <input type="password" id="password-field">
                    <div class="button">Log In</div>
                </div>
            `,
			want:    true,
			wantErr: false,
		},
		{
			name: "Fallback with user name heuristic",
			htmlContent: `
                <input type="text" name="login_user">
                <input type="password" name="login_pass">
            `,
			want:    true,
			wantErr: false,
		},
		{
			name: "No Form Elements",
			htmlContent: `
                <h1>Welcome</h1>
                <p>Please log in on the other page.</p>
            `,
			want:    false,
			wantErr: false,
		},
		{
			name:        "Empty Document",
			htmlContent: ``,
			want:        false,
			wantErr:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.htmlContent))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			got, err := detectLoginForm(ctx, logger, doc)

			if (err != nil) != tc.wantErr {
				t.Errorf("detectLoginForm() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("detectLoginForm() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsValidURL(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "Valid HTTPS URL", input: "https://www.google.com", expected: true},
		{name: "Valid HTTP URL", input: "http://example.org", expected: true},
		{name: "Valid URL with path and query", input: "https://subdomain.example.co.uk/path?query=123", expected: true},
		{name: "Valid URL with port", input: "http://test.com:8080/resource", expected: true},

		{name: "Invalid Scheme (ftp)", input: "ftp://example.com", expected: false},
		{name: "Invalid Scheme (mailto)", input: "mailto:user@example.com", expected: false},
		{name: "Missing Scheme", input: "www.google.com", expected: false},
		{name: "Just a host", input: "google.com", expected: false},
		{name: "Host without a dot", input: "http://localhost", expected: false},
		{name: "Malformed string", input: "this is not a url", expected: false},
		{name: "Empty string", input: "", expected: false},
		{name: "Scheme only", input: "https://", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := isValidURL(tc.input)
			if actual != tc.expected {
				t.Errorf("For input '%s', expected %v, but got %v", tc.input, tc.expected, actual)
			}
		})
	}
}
