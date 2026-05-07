package ats

import "testing"

func TestHTMLToPlainText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"strips inline tags", "<p>Hello <b>world</b></p>", "Hello world"},
		{"decodes entity after tag stripping", "&amp;lt;", "&lt;"},
		{"collapses internal whitespace runs", "<p>  multiple   spaces  </p>", "multiple spaces"},
		{"empty input", "", ""},
		{"all-markup input", "<br/>", ""},

		// Additional coverage for the documented contract.
		{"named entity quot", "&quot;hi&quot;", "\"hi\""},
		{"named entity #39 apostrophe", "it&#39;s", "it's"},
		{"numeric decimal entity", "A&#65;Z", "AAZ"},
		{"numeric hex entity", "&#x41;", "A"},
		{"unknown entity passes through", "&fake;", "&fake;"},
		{"newlines collapse to single space", "line1\n\n\tline2", "line1 line2"},
		{"trims leading and trailing whitespace", "   <p>x</p>   ", "x"},
		{"unclosed tag is treated as literal text", "a < b", "a < b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToPlainText(tc.in)
			if got != tc.want {
				t.Fatalf("htmlToPlainText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
