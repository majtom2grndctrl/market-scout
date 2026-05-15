package main

import (
	"strings"
	"testing"
)

// TestFormatHint_AllKinds verifies every ValidationFailureKind constant
// produces a hint string with the operator-facing wording the retry loop
// depends on. Substring matching keeps the test stable across minor copy
// edits while still pinning the load-bearing tokens.
func TestFormatHint_AllKinds(t *testing.T) {
	cases := []struct {
		name     string
		kind     ValidationFailureKind
		args     []string
		contains []string
	}{
		{
			name:     "invalid_slug quotes the slug in backticks",
			kind:     FailInvalidSlug,
			args:     []string{"Bad_Slug"},
			contains: []string{"`Bad_Slug`", "valid slug", "lowercase"},
		},
		{
			name:     "null_byte names the field",
			kind:     FailNullByte,
			args:     []string{"canonical_role.notes"},
			contains: []string{"`canonical_role.notes`", "null byte"},
		},
		{
			name:     "seniority_invalid mentions the closed set",
			kind:     FailSeniorityInvalid,
			args:     []string{"ultra"},
			contains: []string{"`ultra`", "not a valid seniority", "intern", "unknown"},
		},
		{
			name:     "seniority_missing takes no args",
			kind:     FailSeniorityMissing,
			args:     nil,
			contains: []string{"seniority is required", "unknown"},
		},
		{
			name:     "empty_dimensions names the role slug",
			kind:     FailEmptyDimensions,
			args:     []string{"software-engineer"},
			contains: []string{"`software-engineer`", "no dimensions"},
		},
		{
			name:     "unknown_dimension lists the closed set",
			kind:     FailUnknownDimension,
			args:     []string{"moonshot", "design, engineering, ic"},
			contains: []string{"`moonshot`", "not a known dimension", "design, engineering, ic"},
		},
		{
			name:     "cross_table_collision names the owning table",
			kind:     FailCrossTableCollision,
			args:     []string{"frontend", "specializations"},
			contains: []string{"`frontend`", "specializations"},
		},
		{
			name:     "notes_too_long names the field",
			kind:     FailNotesTooLong,
			args:     []string{"classification.notes"},
			contains: []string{"`classification.notes`", "too long", "4096"},
		},
		{
			name:     "duplicate_slug quotes the slug",
			kind:     FailDuplicateSlug,
			args:     []string{"frontend"},
			contains: []string{"`frontend`", "more than once", "unique"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatHint(tc.kind, tc.args...)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("hint %q missing %q", got, want)
				}
			}
		})
	}
}

func TestRenderRetryPrompt_AppendsGuidanceBlock(t *testing.T) {
	original := "# Posting 1: Software Engineer\n\nbody"
	hints := []string{"hint one", "hint two"}

	got := RenderRetryPrompt(original, hints)

	if !strings.HasPrefix(got, original) {
		t.Errorf("expected retry prompt to begin with original prompt")
	}
	if !strings.Contains(got, "## Retry guidance") {
		t.Errorf("expected '## Retry guidance' heading, got %q", got)
	}
	// Each hint should appear on its own line as a bullet.
	for _, h := range hints {
		bullet := "- " + h
		if !strings.Contains(got, bullet) {
			t.Errorf("expected bullet %q in retry prompt, got %q", bullet, got)
		}
	}
	// Bullets should be on separate lines.
	lines := strings.Split(got, "\n")
	var hintLines int
	for _, line := range lines {
		if strings.HasPrefix(line, "- hint ") {
			hintLines++
		}
	}
	if hintLines != 2 {
		t.Errorf("expected 2 hint lines, got %d", hintLines)
	}
}

// newPromptTaxonomy returns a taxonomy with multiple entries per category so
// the prompt rendering test can verify every slug shows up in the output.
func newPromptTaxonomy() Taxonomy {
	t := Taxonomy{
		CanonicalRoles: map[string]TaxonomyEntry{
			"software-engineer": {ID: 1, Name: "Software Engineer"},
			"product-designer":  {ID: 2, Name: "Product Designer"},
		},
		Specializations: map[string]TaxonomyEntry{
			"frontend": {ID: 10, Name: "Frontend"},
			"backend":  {ID: 11, Name: "Backend"},
		},
		Skills: map[string]TaxonomyEntry{
			"typescript": {ID: 20, Name: "TypeScript"},
			"go":         {ID: 21, Name: "Go"},
		},
		RoleDimensions: map[string]TaxonomyEntry{
			"ic":          {ID: 30, Name: "Individual Contributor"},
			"engineering": {ID: 31, Name: "Engineering"},
		},
	}
	t = t.BuildCrossTableIndex()
	return t
}

func TestRenderSystemPrompt_ContainsTaxonomyAndContract(t *testing.T) {
	tax := newPromptTaxonomy()

	out := RenderSystemPrompt(tax, "")

	// Role dimensions heading and each section heading.
	wantHeadings := []string{
		"## Role dimensions (closed set)",
		"## Canonical roles (existing)",
		"## Specializations (existing)",
		"## Skills (existing)",
	}
	for _, h := range wantHeadings {
		if !strings.Contains(out, h) {
			t.Errorf("expected heading %q in system prompt", h)
		}
	}

	// Every taxonomy slug should appear.
	allSlugs := []string{
		"software-engineer", "product-designer",
		"frontend", "backend",
		"typescript", "go",
		"ic", "engineering",
	}
	for _, slug := range allSlugs {
		if !strings.Contains(out, slug) {
			t.Errorf("expected slug %q in system prompt", slug)
		}
	}

	// Agent contract block (copied verbatim from SKILL.md §7).
	wantContract := []string{
		"## Agent contract",
		"Return JSON only",
		"Classification discipline:",
		"Slug discipline:",
		"Summary contract:",
	}
	for _, c := range wantContract {
		if !strings.Contains(out, c) {
			t.Errorf("expected contract marker %q in system prompt", c)
		}
	}
}

func TestRenderSystemPrompt_FocusBlockPresenceTracksFocus(t *testing.T) {
	tax := newPromptTaxonomy()

	withFocus := RenderSystemPrompt(tax, "golang")
	if !strings.Contains(withFocus, "## Focus guidance") {
		t.Errorf("expected focus guidance heading when focus is set")
	}
	if !strings.Contains(withFocus, "golang") {
		t.Errorf("expected focus token 'golang' in system prompt")
	}

	withoutFocus := RenderSystemPrompt(tax, "")
	if strings.Contains(withoutFocus, "## Focus guidance") {
		t.Errorf("expected focus block to be absent when focus is empty")
	}

	// Whitespace-only focus also suppresses the block (per RenderSystemPrompt impl).
	withWhitespace := RenderSystemPrompt(tax, "   ")
	if strings.Contains(withWhitespace, "## Focus guidance") {
		t.Errorf("expected focus block to be absent for whitespace-only focus")
	}
}

// TestRenderSystemPrompt_TaxonomySectionOrder verifies that taxonomy sections
// appear in the canonical layout agents are tuned to. Reordering sections
// would break agents without failing any content-presence test.
func TestRenderSystemPrompt_TaxonomySectionOrder(t *testing.T) {
	tax := newPromptTaxonomy()

	rendered := RenderSystemPrompt(tax, "")

	rolesIdx := strings.Index(rendered, "## Canonical roles (existing)")
	dimsIdx := strings.Index(rendered, "## Role dimensions (closed set)")
	specsIdx := strings.Index(rendered, "## Specializations (existing)")
	skillsIdx := strings.Index(rendered, "## Skills (existing)")

	if rolesIdx < 0 || dimsIdx < 0 || specsIdx < 0 || skillsIdx < 0 {
		t.Fatal("missing taxonomy section heading")
	}
	if !(rolesIdx < dimsIdx && dimsIdx < specsIdx && specsIdx < skillsIdx) {
		t.Errorf("taxonomy sections out of order: roles=%d dims=%d specs=%d skills=%d",
			rolesIdx, dimsIdx, specsIdx, skillsIdx)
	}
}

// TestRenderUserMessage_ContainsPostingHeaderAndDescription pins the user
// message shape: a "# Posting <id>: <title>" heading and the description
// body. Taxonomy and contract live in the system prompt and must not leak
// into the user message — that would defeat prompt caching.
func TestRenderUserMessage_ContainsPostingHeaderAndDescription(t *testing.T) {
	posting := SelectedPosting{
		PostingID:       42,
		Title:           "Software Engineer",
		DescriptionText: "We are looking for an experienced engineer.",
	}

	out := RenderUserMessage(posting)

	if !strings.Contains(out, "# Posting 42: Software Engineer") {
		t.Errorf("expected posting heading in user message, got %q", out)
	}
	if !strings.Contains(out, "## Description") {
		t.Errorf("expected description heading in user message")
	}
	if !strings.Contains(out, posting.DescriptionText) {
		t.Errorf("expected description body in user message")
	}

	// Taxonomy/contract content must not appear here.
	mustNotContain := []string{
		"## Canonical roles (existing)",
		"## Role dimensions (closed set)",
		"## Agent contract",
	}
	for _, s := range mustNotContain {
		if strings.Contains(out, s) {
			t.Errorf("user message should not contain %q (belongs in system prompt)", s)
		}
	}
}
