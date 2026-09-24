package main

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

func sel(id, company int64, title, desc string) selection.Posting {
	return selection.Posting{PostingID: id, CompanyID: company, Title: title, DescriptionText: desc}
}

// normalize lowercases and collapses whitespace, so "contains" checks cannot
// be dodged by casing or spacing.
func normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func TestMaskTitle(t *testing.T) {
	cases := []struct {
		name, title, desc string
	}{
		{"body repeats the full title", "Senior Software Engineer, Payments",
			"As a Senior Software Engineer, Payments you will build payments. The senior software engineer - payments owns ledgers."},
		{"body uses the level-stripped title", "Staff Product Designer",
			"Our Product Designer shapes the app. Product designers here pair with research."},
		{"title repeated back to back", "Data Engineer",
			"Data Engineer Data Engineer Data Engineer wanted."},
		{"head of strips its connector", "Head of Design",
			"The Head of Design leads Design across the company."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := normalize(maskTitle(tc.desc, tc.title))
			for _, target := range maskTargets(tc.title) {
				words := wordRun.FindAllString(strings.ToLower(target), -1)
				if strings.Contains(state, strings.Join(words, " ")) {
					t.Errorf("state still contains %q: %s", target, state)
				}
			}
			if strings.Contains(state, normalize(tc.title)) {
				t.Errorf("state still contains the title: %s", state)
			}
		})
	}

	t.Run("masks title text glued to a neighbouring word or inside a longer form", func(t *testing.T) {
		got := normalize(maskTitle("The RoleAs a Senior Software EngineerYou'll lead software engineering.", "Senior Software Engineer"))
		for _, leaked := range []string{"senior software engineer", "software engineer"} {
			if strings.Contains(got, leaked) {
				t.Errorf("state still contains %q: %s", leaked, got)
			}
		}
		if !strings.Contains(got, "you'll lead") {
			t.Errorf("masking ate the neighbouring text: %s", got)
		}
	})
}

func TestRoleTournament(t *testing.T) {
	tax := taxonomyWithRoles(120)
	opts, err := newRoleOptions(tax)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("every live role is in exactly one pass-1 question, by name only", func(t *testing.T) {
		req := pass1Request(DefaultModel, "state", opts, nil)
		if len(req.Questions) != 3 {
			t.Fatalf("pass 1 has %d questions for 120 roles, want 3 chunks of at most %d", len(req.Questions), maxChunkSize)
		}
		seen := map[string]int{}
		for id, q := range req.Questions {
			if _, ok := q.Criteria[noneFit]; !ok {
				t.Errorf("%s offers no none_fit", id)
			}
			for opt, desc := range q.Criteria {
				if opt == noneFit {
					continue
				}
				seen[opt]++
				if desc != nil {
					t.Errorf("%s offers %q with a description", id, opt)
				}
				if strings.HasPrefix(opt, "role-") {
					t.Errorf("%s offers a slug %q", id, opt)
				}
			}
		}
		for _, e := range tax.CanonicalRoles {
			if seen[e.Name] != 1 {
				t.Errorf("%q appears in %d pass-1 questions", e.Name, seen[e.Name])
			}
		}
	})

	t.Run("the same role set produces the same chunks", func(t *testing.T) {
		again, _ := newRoleOptions(taxonomyWithRoles(120))
		if !slices.EqualFunc(opts.chunks, again.chunks, slices.Equal) {
			t.Error("chunks differ across two reads of one role set")
		}
	})

	t.Run("pass 2 offers each chunk's leaders plus none_fit", func(t *testing.T) {
		d := newFakeDecider()
		res, _, err := runTournament(t.Context(), newSharedDecider(d), DefaultModel, "hiring a Job Family 042", opts, nil)
		if err != nil {
			t.Fatal(err)
		}
		p2 := d.requests[1].Questions[pass2QuestionID]
		if got, want := len(p2.Criteria), 3*leadersPerChunk+1; got != want {
			t.Errorf("pass 2 offers %d options, want %d", got, want)
		}
		if _, ok := p2.Criteria["Job Family 042"]; !ok {
			t.Error("pass 2 lost the chunk leader")
		}
		if _, ok := p2.Criteria[noneFit]; !ok {
			t.Error("pass 2 offers no none_fit")
		}
		if top, ok := res.top(); !ok || top.Slug != "role-042" {
			t.Errorf("top = %+v, %v", top, ok)
		}
		if len(res.Distribution) != 3*leadersPerChunk {
			t.Errorf("distribution keeps %d roles, want the full pass-2 set", len(res.Distribution))
		}
	})

	t.Run("a role minted after options are read is not offered in that run (P14)", func(t *testing.T) {
		live := taxonomyWithRoles(60)
		d := newFakeDecider()
		deps := testDeps(live, d, sel(1, 1, "Maker", "hiring a Job Family 001"))
		inner := deps.selectPostings
		deps.selectPostings = func(ctx context.Context) ([]selection.Posting, error) {
			live.CanonicalRoles["minted-mid-run"] = classify.TaxonomyEntry{ID: 999, Name: "Minted Mid Run"}
			return inner(ctx)
		}
		runDry(t, deps)
		for _, r := range d.requests {
			for _, q := range r.Questions {
				if _, ok := q.Criteria["Minted Mid Run"]; ok {
					t.Fatal("a role minted after the option read was offered")
				}
			}
		}
	})
}

func TestRoleDefer(t *testing.T) {
	tax := taxonomyWithRoles(60)
	got, _, _ := runDry(t, testDeps(tax, newFakeDecider(),
		sel(1, 1, "Maker", "NONEFIT: we sell sandwiches"),
		sel(2, 1, "Maker", "LOW: hiring a Job Family 007"),
		sel(3, 1, "Maker", "hiring a Job Family 007"),
	))
	for id, want := range map[int64]struct {
		outcome outcome
		reason  string
	}{
		1: {outcomeDeferred, reasonNoneFit},
		2: {outcomeDeferred, reasonBelowRoleFloor},
		3: {outcomeWouldWrite, ""},
	} {
		r := got[id]
		if r.Outcome != want.outcome || r.Reason != want.reason {
			t.Errorf("posting %d: %s/%q, want %s/%q", id, r.Outcome, r.Reason, want.outcome, want.reason)
		}
		if (r.Role != nil) != (want.outcome == outcomeWouldWrite) {
			t.Errorf("posting %d: role %+v on outcome %s", id, r.Role, r.Outcome)
		}
		if len(r.RoleDistribution) == 0 {
			t.Errorf("posting %d kept no candidates", id)
		}
	}
}

func TestPostingAtomicity(t *testing.T) {
	got, summary, _ := runDry(t, testDeps(taxonomyWithRoles(60), newFakeDecider(),
		sel(1, 1, "Maker", "FAIL hiring a Job Family 003"),
		sel(2, 1, "Maker", "hiring a Job Family 004"),
	))
	if r := got[1]; r.Outcome != outcomeFailed || r.Role != nil || len(r.RoleDistribution) != 0 {
		t.Errorf("failed posting kept partial results: %+v", r)
	}
	if got[2].Outcome != outcomeWouldWrite {
		t.Errorf("the other posting did not proceed: %+v", got[2])
	}
	if summary.Counts[outcomeFailed] != 1 || summary.Counts[outcomeWouldWrite] != 1 {
		t.Errorf("counts = %v", summary.Counts)
	}
}

func TestRequestSharing(t *testing.T) {
	tax := taxonomyWithRoles(60)
	const text = "hiring a Job Family 010 to\nbuild   things"

	t.Run("whitespace-only twins share one request and one answer (P6)", func(t *testing.T) {
		d := newFakeDecider()
		got, _, _ := runDry(t, testDeps(tax, d,
			sel(1, 1, "Maker", text),
			sel(2, 2, "Maker", strings.ReplaceAll(text, " ", "\t ")),
		))
		if d.calls() != 2 {
			t.Errorf("sent %d requests, want 2 (one pass 1, one pass 2)", d.calls())
		}
		if !slices.Equal(got[1].RequestKeys, got[2].RequestKeys) || got[1].Role.Slug != got[2].Role.Slug {
			t.Errorf("twins answered differently: %+v vs %+v", got[1], got[2])
		}
	})

	t.Run("one differing character sends its own requests (P6)", func(t *testing.T) {
		d := newFakeDecider()
		runDry(t, testDeps(tax, d, sel(1, 1, "Maker", text), sel(2, 2, "Maker", text+"!")))
		if d.calls() != 4 {
			t.Errorf("sent %d requests, want 4", d.calls())
		}
	})

	t.Run("two runs over one posting each send their own requests (P5)", func(t *testing.T) {
		d := newFakeDecider()
		deps := testDeps(tax, d, sel(1, 1, "Maker", text))
		runDry(t, deps)
		runDry(t, deps)
		if d.calls() != 4 {
			t.Errorf("sent %d requests over two runs, want 4", d.calls())
		}
	})

	t.Run("a failed shared request fails every sharer (P7)", func(t *testing.T) {
		d := newFakeDecider()
		got, _, _ := runDry(t, testDeps(tax, d,
			sel(1, 1, "Maker", "FAIL "+text),
			sel(2, 2, "Maker", "FAIL  "+text),
		))
		for id, r := range got {
			if r.Outcome != outcomeFailed {
				t.Errorf("posting %d: %s, want failed", id, r.Outcome)
			}
		}
		if d.calls() != 1 {
			t.Errorf("sent %d requests, want the one shared failure", d.calls())
		}
	})
}

func TestDryRun_JSONL(t *testing.T) {
	d := newFakeDecider()
	_, summary, lines := runDry(t, testDeps(taxonomyWithRoles(60), d,
		sel(1, 1, "Maker", "hiring a Job Family 020"),
		sel(2, 1, "Maker", "NONEFIT"),
		sel(3, 2, "Maker", "   "),
	))

	if lines[0]["type"] != "run" || lines[0]["prompt_version"] != PromptVersion || lines[0]["dry_run"] != true {
		t.Errorf("header = %v", lines[0])
	}
	if last := lines[len(lines)-1]; last["type"] != "summary" {
		t.Errorf("last line = %v, want the summary", last)
	}
	postings := 0
	for _, l := range lines {
		if l["type"] != "posting" {
			continue
		}
		postings++
		if _, ok := l["outcome"]; !ok {
			t.Errorf("posting line has no outcome: %v", l)
		}
		if l["outcome"] == string(outcomeWouldWrite) {
			role := l["role"].(map[string]any)
			if role["method"] != methodJev || role["probability"] == nil {
				t.Errorf("role label lacks instrument or probability: %v", role)
			}
			if len(l["request_keys"].([]any)) != 2 {
				t.Errorf("request keys = %v", l["request_keys"])
			}
			if l["model"] != d.model {
				t.Errorf("model = %v, want the dated id the response reported", l["model"])
			}
		}
	}
	if postings != 3 {
		t.Errorf("%d posting lines for 3 selected postings", postings)
	}
	want := map[outcome]int{outcomeWouldWrite: 1, outcomeDeferred: 1, outcomeSkipped: 1}
	if !maps.Equal(summary.Counts, want) {
		t.Errorf("counts = %v, want %v", summary.Counts, want)
	}
	if summary.Usage.Requests != 4 {
		t.Errorf("usage requests = %d, want 4", summary.Usage.Requests)
	}
}

func TestNewRoleOptions_RejectsAmbiguousNames(t *testing.T) {
	for name, tax := range map[string]classify.Taxonomy{
		"names equal after case and spacing": {CanonicalRoles: map[string]classify.TaxonomyEntry{
			"a": {Name: "Data  Engineer"}, "b": {Name: "data engineer"}}},
		"a role named like the abstain option": {CanonicalRoles: map[string]classify.TaxonomyEntry{
			"a": {Name: "None_Fit"}}},
		"no roles": {CanonicalRoles: map[string]classify.TaxonomyEntry{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newRoleOptions(tax); err == nil {
				t.Error("accepted")
			}
		})
	}
}

func TestChunkSizes(t *testing.T) {
	for _, n := range []int{1, 50, 51, 291, 600} {
		opts, err := newRoleOptions(taxonomyWithRoles(n))
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		for _, c := range opts.chunks {
			if len(c) > maxChunkSize {
				t.Errorf("n=%d: chunk of %d", n, len(c))
			}
			total += len(c)
		}
		if total != n {
			t.Errorf("n=%d: chunks hold %d roles", n, total)
		}
		if n == 291 && len(opts.chunks) != 6 {
			t.Errorf("291 roles make %d chunks, want 6", len(opts.chunks))
		}
	}
}

func labelsBy(ls []label) map[string]string {
	out := map[string]string{}
	for _, l := range ls {
		out[l.Slug] = l.Method
	}
	return out
}

func TestDimensions(t *testing.T) {
	tax := taxonomyWithRoles(60)

	t.Run("each dimension uses its instrument and rules", func(t *testing.T) {
		got, _, _ := runDry(t, testDeps(tax, newFakeDecider(),
			sel(1, 1, "Maker", "hiring a Job Family 001 who writes Python NOUL:payments NOUL:ai-agents=0.3 NOUL:stakeholder-management"),
		))
		r := got[1]
		if !maps.Equal(labelsBy(r.Specializations), map[string]string{"payments": methodJev}) {
			t.Errorf("specializations = %v", labelsBy(r.Specializations))
		}
		if !maps.Equal(labelsBy(r.Skills), map[string]string{"python": methodLexical, "stakeholder-management": methodJev}) {
			t.Errorf("skills = %v", labelsBy(r.Skills))
		}
		recordedSlugs := map[string]float64{}
		for _, c := range r.Candidates {
			recordedSlugs[c.Dimension+":"+c.Slug] = c.Probability
		}
		if recordedSlugs["specialization:ai-agents"] != 0.3 {
			t.Errorf("a dropped label above the recording floor was not kept as a candidate: %v", recordedSlugs)
		}
		if _, ok := recordedSlugs["skill:problem-solving"]; ok {
			t.Error("a candidate below the recording floor was kept")
		}
	})

	t.Run("a deferred posting keeps its candidates and gets no labels", func(t *testing.T) {
		got, _, _ := runDry(t, testDeps(tax, newFakeDecider(), sel(1, 1, "Maker", "NONEFIT NOUL:payments Python")))
		r := got[1]
		if r.Outcome != outcomeDeferred || len(r.Candidates) == 0 || r.Specializations != nil || r.Skills != nil {
			t.Errorf("deferred posting: %+v", r)
		}
	})

	t.Run("a posting with no surviving specialization or skill still writes, arrays empty", func(t *testing.T) {
		got, _, _ := runDry(t, testDeps(tax, newFakeDecider(), sel(1, 1, "Maker", "hiring a Job Family 001")))
		r := got[1]
		if r.Outcome != outcomeWouldWrite || len(r.Specializations) != 0 || len(r.Skills) != 0 {
			t.Errorf("zero-survivor posting: %+v", r)
		}
	})

	t.Run("a slug in two taxonomy tables is offered nowhere", func(t *testing.T) {
		d := newFakeDecider()
		deps := testDeps(tax, d, sel(1, 1, "Maker", "hiring a Job Family 001"))
		deps.crossTable = func(context.Context) ([]string, error) { return []string{"payments", "stakeholder-management"}, nil }
		runDry(t, deps)
		for id := range d.requests[0].Questions {
			if id == specQuestionID("payments") || id == skillQuestionID("stakeholder-management") {
				t.Errorf("offered cross-table slug %s", id)
			}
		}
	})

	t.Run("noul questions ride on the pass-1 request, by name", func(t *testing.T) {
		d := newFakeDecider()
		runDry(t, testDeps(tax, d, sel(1, 1, "Maker", "hiring a Job Family 001")))
		if d.calls() != 2 {
			t.Errorf("sent %d requests, want pass 1 and pass 2 only", d.calls())
		}
		q := d.requests[0].Questions[skillQuestionID("problem-solving")]
		if q.Type != "noul" || !strings.Contains(q.Instructions, "problem-solving") {
			t.Errorf("skill noul = %+v", q)
		}
	})
}

func TestSeniority_Pipeline(t *testing.T) {
	tax := taxonomyWithRoles(60)
	cases := []struct {
		name, title, desc string
		value, note       string
	}{
		{"a title level word acting as a level", "Senior Widget Engineer",
			"hiring a Job Family 001. You will partner with senior stakeholders. NOTLEVEL:stakeholders",
			"senior", `seniority[step1-title]: "Senior"`},
		{"a body level word acting as a level", "Widget Engineer",
			"hiring a Job Family 001 at the Staff level for our platform.",
			"staff", `seniority[step1-body]: "Staff"`},
		{"senior stakeholders is judged not a level", "Widget Engineer",
			"hiring a Job Family 001. You will partner with senior stakeholders. NOTLEVEL:stakeholders",
			"unknown", ""},
		{"Chief of Staff carries no level", "Chief of Staff",
			"hiring a Job Family 001 to run the office. NOTLEVEL:Chief",
			"unknown", ""},
		{"no candidate is unknown with no note", "Widget Engineer",
			"hiring a Job Family 001 to build widgets.",
			"unknown", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _ := runDry(t, testDeps(tax, newFakeDecider(), sel(1, 1, tc.title, tc.desc)))
			r := got[1]
			if r.Seniority != tc.value || r.SeniorityNote != tc.note {
				t.Errorf("seniority = %q, note %q; want %q, %q", r.Seniority, r.SeniorityNote, tc.value, tc.note)
			}
		})
	}

	t.Run("seniority reads the title in its own request; the role requests never do", func(t *testing.T) {
		d := newFakeDecider()
		runDry(t, testDeps(tax, d, sel(1, 1, "Senior Widget Engineer", "hiring a Job Family 001 as a Senior Widget Engineer.")))
		if d.calls() != 3 {
			t.Fatalf("sent %d requests, want pass 1, pass 2, and seniority", d.calls())
		}
		for _, r := range d.requests {
			_, isSeniority := r.Questions["seniority_001"]
			hasTitle := strings.Contains(strings.ToLower(r.State), "widget engineer")
			if isSeniority != hasTitle {
				t.Errorf("request with seniority=%v carries the title=%v: %q", isSeniority, hasTitle, r.State)
			}
		}
	})

	t.Run("the same phrase in the same sentence is asked once", func(t *testing.T) {
		d := newFakeDecider()
		runDry(t, testDeps(tax, d, sel(1, 1, "Widget Engineer", "hiring a Job Family 001. Lead, Lead, Lead. Lead, Lead, Lead.")))
		var seniorityReq int
		for _, r := range d.requests {
			if _, ok := r.Questions["seniority_001"]; ok {
				seniorityReq++
				if len(r.Questions) != 1 {
					t.Errorf("asked %d questions for one repeated phrase", len(r.Questions))
				}
			}
		}
		if seniorityReq != 1 {
			t.Errorf("%d seniority requests", seniorityReq)
		}
	})
}
