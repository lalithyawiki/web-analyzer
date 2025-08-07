package analyzer

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type LinkSummary struct {
	InternalCount     int
	ExternalCount     int
	InaccessibleCount int
}

type AnalysisResult struct {
	HTMLVersion       string
	Title             string
	Headings          map[string]int
	Links             LinkSummary
	ContainsLoginForm bool
}

// TODO: structure this file
func AnalyzePage(pageURL string) (*AnalysisResult, error) {
	data, err := http.Get(pageURL)

	if err != nil {
		fmt.Print("Hi")
		fmt.Print(err)
	}

	defer data.Body.Close()

	doc, err := goquery.NewDocumentFromReader(data.Body)
	if err != nil {
		fmt.Println((err))
	}

	result := &AnalysisResult{
		Headings: make(map[string]int),
	}

	doc.Each(func(i int, s *goquery.Selection) {
		for _, node := range s.Nodes {
			if node.FirstChild != nil && node.FirstChild.Type == html.DoctypeNode {
				doctype := node.FirstChild

				if doctype.Data == "html" && len(doctype.Attr) == 0 {
					result.HTMLVersion = "HTML5"
					return
				}

				for _, attr := range doctype.Attr {
					if attr.Key == "public" {
						val := strings.ToLower(attr.Val)
						if strings.Contains(val, "xhtml 1.0") {
							result.HTMLVersion = "XHTML 1.0"
						} else if strings.Contains(val, "html 4.01") {
							result.HTMLVersion = "HTML 4.01"
						} else {
							result.HTMLVersion = "Unknown (Pre-HTML5)"
						}
						return
					}
				}
			}
		}
	})

	result.Title = doc.Find("title").Text()

	headingLevels := []string{"h1", "h2", "h3", "h4", "h5", "h6"}
	for _, tag := range headingLevels {
		count := doc.Find(tag).Length()
		if count > 0 {
			result.Headings[tag] = count
		}
	}

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		href = strings.TrimSpace(href)

		if href == "" || strings.HasPrefix(href, "#") {
			return
		}

		if strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			return
		}

		linkURL, err := url.Parse(href)
		if err != nil {
			return
		}

		// TODO: Check original urls protocol here
		if linkURL.Host != "" && linkURL.Host != "https" {
			result.Links.ExternalCount++
		} else {
			result.Links.InternalCount++
		}
	})

	if doc.Find("input[type='password']").Length() > 0 {
		result.ContainsLoginForm = true
	}

	// TODO: Implement async processing using go routines for this
	result.Links.InaccessibleCount = 10

	return result, nil
}
