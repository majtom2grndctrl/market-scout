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

		// script/style element bodies must not appear in output.
		{"script body is dropped", `before<script>alert("hi")</script>after`, "beforeafter"},
		{"style body is dropped", `before<style>body{color:red}</style>after`, "beforeafter"},
		{"script with attributes is dropped", `<script type="text/javascript">x=1</script>`, ""},
		{"style uppercase tag is dropped", `<STYLE>.foo{}</STYLE>`, ""},
		{"script mixed-case tag is dropped", `<Script>bad()</Script>`, ""},

		// Numeric entities that map to control characters must be suppressed.
		{"NUL entity is suppressed", "a&#0;b", "ab"},
		{"BEL entity is suppressed", "a&#7;b", "ab"},
		{"SOH entity is suppressed", "&#1;", ""},
		{"DEL entity is suppressed", "&#127;b", "b"},
		{"C1 entity is suppressed", "&#x80;b", "b"},
		// Tab/LF/CR entities are decoded but then collapsed to a single space
		// by the whitespace normalization pass (same as literal whitespace).
		{"tab entity collapses to space", "a&#9;b", "a b"},
		{"LF entity collapses to space", "a&#10;b", "a b"},
		{"CR entity collapses to space", "a&#13;b", "a b"},
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
