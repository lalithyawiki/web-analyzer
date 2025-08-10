package analyzer

type LinkSummary struct {
	InternalCount     int
	ExternalCount     int
	InaccessibleCount int
}

type LinkAnalysis struct {
	InternalLinks []string
	ExternalLinks []string
}

type AnalysisResult struct {
	HTMLVersion       string
	Title             string
	Headings          map[string]int
	Links             LinkSummary
	ContainsLoginForm bool
}
