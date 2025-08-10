package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/PuerkitoBio/goquery"
)

func AnalyzePage(ctx context.Context, logger *slog.Logger, pageURL string) (*AnalysisResult, error) {
	logger = logger.With(slog.String("Analyzing page url", pageURL))
	logger.DebugContext(ctx, "Starting page analysis")

	// --- 1. Load Web Page ---
	data, err := loadWebPage(ctx, logger, pageURL)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to load web page", slog.Any("error", err))
		return nil, err
	}
	defer data.Body.Close()

	doc, err := goquery.NewDocumentFromReader(data.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to parse HTML document", slog.Any("error", err))
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	result := &AnalysisResult{
		Headings: make(map[string]int),
	}

	// --- 2. Run All Analyses ---
	logger.DebugContext(ctx, "Beginning individual analyses")

	// HTML Version
	result.HTMLVersion, _ = findHTMLVersion(ctx, logger, doc)

	// Title
	result.Title = doc.Find("title").Text()

	// Heading Counts
	result.Headings, _ = countHeadings(ctx, logger, doc)

	// Link Extraction
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		logger.ErrorContext(ctx, "Fatal: could not parse base URL", slog.Any("error", err))
		return nil, fmt.Errorf("could not parse base URL: %w", err)
	}
	linkAnalysis, _ := extractLinks(ctx, logger, doc, baseURL)
	result.Links.InternalCount = len(linkAnalysis.InternalLinks)
	result.Links.ExternalCount = len(linkAnalysis.ExternalLinks)

	// Login Form Detection
	result.ContainsLoginForm, _ = detectLoginForm(ctx, logger, doc)

	// Inaccessible Link Check
	failedLinks, _ := validateLinkAccessibility(ctx, logger, linkAnalysis)
	result.Links.InaccessibleCount = len(failedLinks)

	// --- 3. Final Summary Log ---
	logger.InfoContext(ctx, "Page analysis complete",
		slog.Group("results",
			slog.String("html_version", result.HTMLVersion),
			slog.String("title", result.Title),
			slog.Int("internal_links", result.Links.InternalCount),
			slog.Int("external_links", result.Links.ExternalCount),
			slog.Int("inaccessible_links", result.Links.InaccessibleCount),
			slog.Bool("has_login_form", result.ContainsLoginForm),
		),
	)

	return result, nil
}
