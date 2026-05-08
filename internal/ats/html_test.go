package ats

import "testing"

func TestHTMLToPlainText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"strips inline tags", "<p>Hello <b>world</b></p>", "Hello world"},
		// See htmlToPlainText godoc for the loop-until-stable rationale; the
		// `&amp;lt;` → `<` collapse is the documented tradeoff.
		{"double-decoded entity collapses to bare char", "&amp;lt;", "<"},
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

		// Greenhouse delivers entity-encoded HTML in its `content` field. The
		// pipeline must decode entities first so the wrapped tags are exposed
		// for stripping; otherwise raw markup leaks into description_text.
		{
			"greenhouse double-encoded html is fully flattened",
			`&lt;div class=&quot;content-intro&quot;&gt;&lt;p&gt;Hello &amp;amp; goodbye&lt;/p&gt;&lt;/div&gt;`,
			"Hello & goodbye",
		},
		{
			"single-encoded html with entity in body still works",
			`<p>Hello &amp; world</p>`,
			"Hello & world",
		},

		// Triple-encoded <script> must not bypass the drop step: the loop
		// peels successive entity layers until the literal tag surfaces and
		// gets dropped along with its body.
		{
			"triple-encoded script body is dropped",
			`&amp;lt;script&amp;gt;alert(1)&amp;lt;/script&amp;gt;after`,
			"after",
		},
		// Triple-encoded `&amp;amp;amp;` unwinds to a single `&` once the loop
		// stabilizes. Locks in idempotency on entity-only input.
		{"triple-encoded amp collapses to single &", "&amp;amp;amp;", "&"},
		// Entity adjacent to a tag boundary: `&amp;` inside `<p>` decodes to
		// `&`, the tags are stripped, and the trailing `x` joins on.
		{"entity adjacent to tag boundary", "<p>&amp;</p>x", "&x"},
		// Anchor with an entity-encoded query separator: the `&amp;` inside
		// the href is consumed when the tag itself is stripped.
		{
			"anchor with entity-encoded query param",
			`see <a href="?foo=1&amp;bar=2">here</a>`,
			"see here",
		},
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

// TestHTMLToPlainTextIdempotent locks in that running htmlToPlainText on its
// own output is a no-op. The loop-until-stable pipeline guarantees this; a
// regression here would mean a fixed-depth pipeline crept back in.
func TestHTMLToPlainTextIdempotent(t *testing.T) {
	inputs := []string{
		// Greenhouse double-encoded HTML.
		`&lt;div class=&quot;content-intro&quot;&gt;&lt;p&gt;Hello &amp;amp; goodbye&lt;/p&gt;&lt;/div&gt;`,
		// Lever-style plain HTML.
		`<p>Hello <b>world</b> &amp; friends</p>`,
		// Anchor with entity-encoded query separator (surviving `&amp;` in attribute prose).
		`see <a href="?foo=1&amp;bar=2">here</a>`,
		// Triply-encoded entity-only input.
		`&amp;amp;amp;`,
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			once := htmlToPlainText(in)
			twice := htmlToPlainText(once)
			if once != twice {
				t.Fatalf("not idempotent: htmlToPlainText(%q) = %q, but htmlToPlainText(%q) = %q",
					in, once, once, twice)
			}
		})
	}
}
