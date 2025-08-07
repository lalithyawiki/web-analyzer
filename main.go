package main

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"web-analyzer/analyzer"
)

func main() {
	http.HandleFunc("/", handleRequest)

	slog.Info("Server starting...", "addr", ":8080")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}

type TemplateData struct {
	URL     string
	Error   string
	Results *analyzer.AnalysisResult
}

func clientError(w http.ResponseWriter, status int, message string) {
	http.Error(w, message, status)
}

var tmpl = template.Must(template.ParseFiles("templates/index.html"))

func serverError(w http.ResponseWriter, err error) {
	trace := string(debug.Stack())
	slog.Error("Internal Server Error", "error", err, "trace", trace)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		clientError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	data := TemplateData{}

	if r.Method == http.MethodPost {
		urlToAnalyze := r.FormValue("url")
		data.URL = urlToAnalyze
		results, err := analyzer.AnalyzePage(urlToAnalyze)
		if err != nil {
			slog.Warn("Analysis failed for URL", "url", urlToAnalyze, "error", err)
			data.Error = "Failed to analyze the page. The URL might be unreachable or the content invalid."
		} else {
			slog.Info("Analysis successful", "url", urlToAnalyze)
			data.Results = results
		}
	}

	err := tmpl.Execute(w, data)

	if err != nil {
		serverError(w, err)
	}
}
