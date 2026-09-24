package seniority

import "testing"

func TestStripLevelWords(t *testing.T) {
	for title, want := range map[string]string{
		"Senior Staff Software Engineer":   "Software Engineer",
		"Sr. Product Designer":             "Product Designer",
		"Senior/Staff Engineer":            "Engineer",
		"Mid-Senior Account Executive":     "Account Executive",
		"Head of Design":                   "Design",
		"VP of Engineering, Platform":      "Engineering, Platform",
		"Entry-level Front-End Engineer":   "Front-End Engineer",
		"Engineering Manager":              "Engineering Manager",
		"Team Leader, Customer Operations": "Team Customer Operations",
		"Internal Tools Engineer":          "Internal Tools Engineer",
		"Staffing Coordinator":             "Staffing Coordinator",
		"Principal":                        "",
	} {
		if got := StripLevelWords(title); got != want {
			t.Errorf("StripLevelWords(%q) = %q, want %q", title, got, want)
		}
	}
}
