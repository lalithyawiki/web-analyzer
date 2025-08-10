package analyzer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

func findHTMLVersion(ctx context.Context, logger *slog.Logger, doc *goquery.Document) (string, error) {
	logger.DebugContext(ctx, "Starting to determine HTML version")

	var version string
	var found bool

	doc.Each(func(i int, s *goquery.Selection) {
		if found {
			return
		}
		for _, node := range s.Nodes {
			if node.FirstChild != nil && node.FirstChild.Type == html.DoctypeNode {
				doctype := node.FirstChild
				logger.DebugContext(ctx, "Found doctype node", slog.String("data", doctype.Data))

				if doctype.Data == "html" && len(doctype.Attr) == 0 {
					version = "HTML5"
					found = true
					return
				}

				for _, attr := range doctype.Attr {
					if attr.Key == "public" {
						val := strings.ToLower(attr.Val)
						logger.DebugContext(ctx, "Found public identifier", slog.String("value", val))
						if strings.Contains(val, "xhtml 1.0") {
							version = "XHTML 1.0"
						} else if strings.Contains(val, "html 4.01") {
							version = "HTML 4.01"
						} else {
							version = "Unknown (Pre-HTML5)"
						}
						found = true
						return
					}
				}
			}
		}
	})

	if version == "" {
		version = "Unknown or No Doctype"
		logger.WarnContext(ctx, "Could not determine HTML version", slog.String("result", version))
	} else {
		logger.InfoContext(ctx, "Successfully determined HTML version", slog.String("version", version))
	}

	return version, nil
}

func countHeadings(ctx context.Context, logger *slog.Logger, doc *goquery.Document) (map[string]int, error) {
	logger.DebugContext(ctx, "Starting to count headings")

	headings := make(map[string]int)
	headingLevels := []string{"h1", "h2", "h3", "h4", "h5", "h6"}

	for _, tag := range headingLevels {
		count := doc.Find(tag).Length()
		if count > 0 {
			logger.DebugContext(
				ctx,
				"Found heading tag",
				slog.String("tag", tag),
				slog.Int("count", count),
			)
			headings[tag] = count
		}
	}

	logger.InfoContext(
		ctx,
		"Successfully counted all headings",
		slog.Any("heading_counts", headings),
	)

	return headings, nil
}

func extractLinks(ctx context.Context, logger *slog.Logger, doc *goquery.Document, baseURL *url.URL) (LinkAnalysis, error) {
	logger = logger.With(slog.String("analyzing_page_link", baseURL.String()))
	logger.DebugContext(ctx, "Starting to extract links")

	result := LinkAnalysis{
		InternalLinks: []string{},
		ExternalLinks: []string{},
	}

	var errs []error

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)

		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			logger.DebugContext(ctx, "Skipping irrelevant link", slog.String("href", href))
			return
		}

		linkURL, err := url.Parse(href)
		if err != nil {
			logger.WarnContext(ctx, "Failed to parse link href", slog.String("href", href), slog.Any("error", err))
			errs = append(errs, fmt.Errorf("failed to parse href '%s': %w", href, err))
			return
		}

		absoluteLink := baseURL.ResolveReference(linkURL)

		if absoluteLink.Host == baseURL.Host {
			logger.DebugContext(ctx, "Found internal link", slog.String("link", absoluteLink.String()))
			result.InternalLinks = append(result.InternalLinks, absoluteLink.String())
		} else {
			logger.DebugContext(ctx, "Found external link", slog.String("link", absoluteLink.String()))
			result.ExternalLinks = append(result.ExternalLinks, absoluteLink.String())
		}
	})

	logger.InfoContext(ctx, "Finished extracting links",
		slog.Int("internal_links_found", len(result.InternalLinks)),
		slog.Int("external_links_found", len(result.ExternalLinks)),
		slog.Int("parsing_errors", len(errs)),
	)

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	return result, nil
}

func detectLoginForm(ctx context.Context, logger *slog.Logger, doc *goquery.Document) (bool, error) {
	logger.DebugContext(ctx, "Starting login form detection")
	var isLoginForm bool

	doc.Find("form").EachWithBreak(func(i int, formSelection *goquery.Selection) bool {
		formLogger := logger.With(slog.Int("form_index", i)) // Create a logger specific to this form
		formLogger.DebugContext(ctx, "Analyzing form")

		// Criterion 1: A password field MUST exist.
		if formSelection.Find("input[type='password']").Length() == 0 {
			formLogger.DebugContext(ctx, "Skipping form: no password field found")
			return true
		}

		// Criterion 2: Check for a username/email field.
		hasUserIdentifierField := false
		formSelection.Find("input").Each(func(j int, inputSelection *goquery.Selection) {
			// Check for various attributes that indicate a user identifier field
			inputType, _ := inputSelection.Attr("type")
			nameAttr := strings.ToLower(inputSelection.AttrOr("name", ""))
			idAttr := strings.ToLower(inputSelection.AttrOr("id", ""))
			placeholderAttr := strings.ToLower(inputSelection.AttrOr("placeholder", ""))

			if inputType == "email" ||
				strings.Contains(nameAttr, "user") || strings.Contains(idAttr, "user") || strings.Contains(placeholderAttr, "user") ||
				strings.Contains(nameAttr, "email") || strings.Contains(idAttr, "email") || strings.Contains(placeholderAttr, "email") {
				hasUserIdentifierField = true
			}
		})

		// Criterion 3: Check for a submit button with "log in" or "sign in" text.
		hasLoginButton := false
		formSelection.Find("button, input[type='submit']").Each(func(k int, btnSelection *goquery.Selection) {
			btnText := strings.ToLower(btnSelection.Text() + btnSelection.AttrOr("value", ""))
			if strings.Contains(btnText, "log in") || strings.Contains(btnText, "sign in") {
				hasLoginButton = true
			}
		})

		formLogger.DebugContext(ctx, "Form analysis criteria",
			slog.Bool("has_user_identifier", hasUserIdentifierField),
			slog.Bool("has_login_button", hasLoginButton),
		)

		if hasUserIdentifierField || hasLoginButton {
			isLoginForm = true
			formLogger.DebugContext(ctx, "Confirmed as login form")
			return false
		}

		return true
	})

	// Fallback for pages without a <form> tag (e.g., logins handled by JavaScript)
	if !isLoginForm {
		logger.DebugContext(ctx, "No traditional login form found, running fallback check")
		hasPasswordInput := doc.Find("input[type='password']").Length() > 0
		hasEmailInput := doc.Find("input[type='email']").Length() > 0
		hasTextInput := doc.Find("input[id*='user'], input[id*='login'], input[name*='user'], input[name*='login']").Length() > 0

		logger.DebugContext(ctx, "Fallback analysis criteria",
			slog.Bool("has_password_input", hasPasswordInput),
			slog.Bool("has_email_input", hasEmailInput),
			slog.Bool("has_text_input_heuristic", hasTextInput),
		)

		if hasPasswordInput && (hasEmailInput || hasTextInput) {
			logger.DebugContext(ctx, "Confirmed as login form via fallback")
			isLoginForm = true
		}
	}

	logger.InfoContext(ctx, "Login form detection finished", slog.Bool("login_form_found", isLoginForm))
	return isLoginForm, nil
}
