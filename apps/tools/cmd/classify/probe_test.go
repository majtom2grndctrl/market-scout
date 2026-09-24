package main

import (
	"strings"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/skillseed"
)

func TestRunHashes(t *testing.T) {
	build := func(tax classify.Taxonomy, seed skillseed.Seed) runHashes {
		o, err := buildOptions(tax, nil, seed)
		if err != nil {
			t.Fatal(err)
		}
		return computeHashes(o)
	}
	base := build(taxonomyWithRoles(60), testSeed())

	if again := build(taxonomyWithRoles(60), testSeed()); again != base {
		t.Errorf("identical inputs hashed differently: %+v vs %+v", again, base)
	}

	roles := taxonomyWithRoles(60)
	roles.CanonicalRoles["role-new"] = classify.TaxonomyEntry{Name: "New Family"}
	specs := taxonomyWithRoles(60)
	specs.Specializations["fintech"] = classify.TaxonomyEntry{Name: "fintech"}
	seed := testSeed()
	seed.Entries = seed.Entries[1:]

	for name, got := range map[string]struct {
		h    runHashes
		move string
	}{
		"a new role":           {build(roles, testSeed()), "role"},
		"a new specialization": {build(specs, testSeed()), "spec"},
		"a seed entry removed": {build(taxonomyWithRoles(60), seed), "seed"},
	} {
		t.Run(name+" moves only its own hash", func(t *testing.T) {
			moved := map[string]bool{
				"role": got.h.RoleOptions != base.RoleOptions,
				"spec": got.h.SpecOptions != base.SpecOptions,
				"seed": got.h.Seed != base.Seed,
				"text": got.h.QuestionText != base.QuestionText,
			}
			for k, m := range moved {
				if m != (k == got.move) {
					t.Errorf("%s hash moved=%v", k, m)
				}
			}
		})
	}

	t.Run("question wording hashes its templates, not the terms", func(t *testing.T) {
		words := questionWording()
		if hashOf(words) != base.QuestionText {
			t.Fatal("question hash is not the hash of the wording")
		}
		words[0] += " "
		if hashOf(words) == base.QuestionText {
			t.Error("a wording change kept the hash")
		}
	})
}

func TestDryRun_HeaderAndSeniorityFields(t *testing.T) {
	_, _, lines := runDry(t, testDeps(taxonomyWithRoles(60), newFakeDecider(),
		sel(1, 1, "Senior Widget Engineer", "hiring a Job Family 001")))
	h := lines[0]
	hashes, _ := h["hashes"].(map[string]any)
	for _, k := range []string{"role_options", "specialization_options", "seed", "question_wording"} {
		if s, _ := hashes[k].(string); len(s) != 64 {
			t.Errorf("header hash %s = %v", k, hashes[k])
		}
	}
	th, _ := h["thresholds"].(map[string]any)
	if th["role_floor"] == nil || th["recording_floor"] == nil || th["specializations"] == nil {
		t.Errorf("header thresholds = %v", th)
	}
	p := lines[1]
	if p["seniority"] != "senior" || !strings.HasPrefix(p["seniority_note"].(string), "seniority[step1-title]") {
		t.Errorf("posting seniority fields = %v / %v", p["seniority"], p["seniority_note"])
	}
}

func TestRoleDescriptionsArm(t *testing.T) {
	opts, _ := newRoleOptions(taxonomyWithRoles(10))
	opts.descriptions = map[string]string{"role-003": "Builds widgets."}
	q := pass1Request(DefaultModel, "s", opts, nil).Questions[pass1QuestionID(0)]
	if d := q.Criteria["Job Family 003"]; d == nil || *d != "Builds widgets." {
		t.Errorf("described role offered as %v", d)
	}
	if d := q.Criteria["Job Family 004"]; d != nil {
		t.Errorf("undescribed role offered with %q", *d)
	}
}

func dist(pairs ...any) []scoredRole {
	var out []scoredRole
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, scoredRole{Slug: pairs[i].(string), Probability: pairs[i+1].(float64)})
	}
	return out
}

func TestProbeReport(t *testing.T) {
	yes, no := true, false
	written := func(id int64, slug string, p float64, rest ...any) postingResult {
		return postingResult{PostingID: id, Outcome: outcomeWouldWrite, Role: &label{Slug: slug},
			RoleDistribution: append(dist(slug, p), dist(rest...)...), NoneFit: 0.01}
	}
	run := dryRunFile{usage: runUsage{Cost: 0.006, Tokens: 60000}, postings: map[int64]postingResult{
		1: written(1, "designer", 0.9),
		2: written(2, "designer", 0.8),
		3: written(3, "analyst", 0.35, "engineer", 0.3),                                                // wrong, low confidence
		4: {PostingID: 4, Outcome: outcomeDeferred, RoleDistribution: dist("chef", 0.2), NoneFit: 0.7}, // right: gold uncovered
		5: written(5, "engineer", 0.5, "manager", 0.3),
		6: written(6, "writer", 0.95),
	}}
	run.postings[1] = func(r postingResult) postingResult {
		r.Skills = []label{{Slug: "python", Method: methodLexical}, {Slug: "figma", Method: methodJev}}
		r.Specializations = []label{{Slug: "payments"}}
		r.Seniority = "senior"
		return r
	}(run.postings[1])
	run2 := dryRunFile{postings: map[int64]postingResult{}}
	for id, r := range run.postings {
		run2.postings[id] = r
	}
	run2.postings[5] = written(5, "manager", 0.5)
	gold := map[int64]goldLine{
		1: {PostingID: 1, Role: "designer", Skills: []string{"python", "sql"}, Specializations: []string{"payments"}, Seniority: "senior", RoleAgreed: &yes},
		2: {PostingID: 2, Role: "designer", RoleAgreed: &yes},
		3: {PostingID: 3, Role: "engineer", RoleAgreed: &no},
		4: {PostingID: 4, RoleUncovered: true, RoleAgreed: &yes},
		5: {PostingID: 5, Role: "engineer", RoleAgreed: &yes},
		6: {PostingID: 6, Role: "writer", RoleAgreed: &yes},
	}
	seed := skillseed.Seed{Entries: []skillseed.Entry{
		{SkillSlug: "python", Reach: skillseed.ReachLexical}, {SkillSlug: "figma", Reach: skillseed.ReachNoul},
	}}
	rep, err := scoreProbe(probeInputs{gold: gold, run: run, run2: &run2, seed: &seed, sample: map[int64]sampleLine{}})
	if err != nil {
		t.Fatal(err)
	}

	check := func(name string, got ratio, hit, of int) {
		t.Helper()
		if got.Hit != hit || got.Of != of {
			t.Errorf("%s = %d/%d, want %d/%d", name, got.Hit, got.Of, hit, of)
		}
	}
	check("top-1", rep.Top1, 5, 6)
	check("top-3", rep.Top3, 5, 5)
	check("gold agreement", rep.GoldAgreement, 5, 6)
	check("flip", rep.Flip, 1, 6)
	check("low third", rep.Thirds[0], 1, 2) // posting 3 (0.35, wrong) and posting 5 (0.5, right)
	check("high third", rep.Thirds[2], 2, 2)
	check("lexical skill precision", rep.SkillP[methodLexical][0], 1, 1)
	check("lexical skill recall over reach", rep.SkillR[methodLexical][0], 1, 1) // sql is unreachable
	check("jev skill precision", rep.SkillP[methodJev][0], 0, 1)
	check("seniority precision", rep.SeniorityP, 1, 1)
	if rep.CostPer1k < 0.99 || rep.CostPer1k > 1.01 {
		t.Errorf("cost per 1k = %v, want $1", rep.CostPer1k)
	}

	var flipKill bool
	for _, k := range rep.Kill {
		flipKill = flipKill || strings.Contains(k, "flip rate")
	}
	if !flipKill {
		t.Errorf("a 16.7%% flip rate did not trip the kill criteria: %v", rep.Kill)
	}
	if !strings.Contains(rep.markdown(), "Kill criteria: STOP") {
		t.Error("report does not lead with the STOP verdict")
	}
}
