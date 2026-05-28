package boilerplate_test

import (
	"strings"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
)

// Long paragraphs (>200 bytes) used to seed shared boilerplate across inputs.
const (
	companyBlurbP1 = "Acme Corporation is a globally recognized leader in distributed systems engineering, serving enterprise customers across forty countries with a relentless commitment to operational excellence and customer success in every region we operate in today."
	companyBlurbP2 = "Founded in 2009, Acme has grown from a small team of five engineers in a Brooklyn loft to a publicly traded company with over twelve hundred employees across nine offices worldwide, all united by a common mission to make infrastructure boring."
	companyBlurbP3 = "Our culture is grounded in radical transparency, written communication, asynchronous collaboration, and a shared belief that the best ideas can come from anywhere in the organization regardless of seniority, tenure, or geographic location."
	companyBlurbP4 = "We invest heavily in employee growth through generous learning stipends, conference budgets, internal mentorship programs, and a four-week paid sabbatical every five years of service so our people can recharge and bring fresh perspective back to the team."

	eeoFooter = "Acme Corporation is an equal opportunity employer. We celebrate diversity and are committed to creating an inclusive environment for all employees regardless of race, color, religion, gender, gender identity or expression, sexual orientation, national origin, genetics, disability, age, or veteran status."
)

func joinParas(paras ...string) string {
	return strings.Join(paras, "\n\n")
}

func TestStrip_AbundantSamples_RemovesSharedBlurb(t *testing.T) {
	roleBodies := []string{
		"You will design and ship the next generation of our scheduling pipeline. Expect to write Go, review pull requests, mentor newer engineers, and partner closely with product on quarterly planning. We are hiring at the senior and staff level for this role.",
		"As a backend engineer on the Billing team, you will own the invoicing service end to end. The role spans on-call rotation, capacity planning, customer-facing incident communication, and incremental modernization of legacy Python services that handle millions of transactions per day.",
		"This position joins our Platform Reliability group. You will be responsible for the kubernetes fleet, the deployment tooling around it, observability across regions, and progressive delivery patterns that let product teams ship safely without paging us at 3am.",
		"Our Data Infrastructure team is hiring an engineer to focus on streaming systems. You will work in Go and Rust, contribute to internal Kafka tooling, design new schemas for change-data-capture pipelines, and partner with analytics on data quality SLAs the business depends on.",
		"Join the Identity team to harden authentication across our product surface. You will lead efforts on passkeys rollout, deprecate legacy SAML flows, and own the hardware-token compliance story for our regulated customers across financial services and healthcare verticals.",
	}

	inputs := make([]string, len(roleBodies))
	for i, body := range roleBodies {
		inputs[i] = joinParas(companyBlurbP1, companyBlurbP2, companyBlurbP3, companyBlurbP4, body)
	}

	got := boilerplate.Strip(inputs)
	if len(got) != len(inputs) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(inputs))
	}

	for i, out := range got {
		for _, blurb := range []string{companyBlurbP1, companyBlurbP2, companyBlurbP3, companyBlurbP4} {
			if strings.Contains(out, blurb) {
				t.Errorf("input %d: expected blurb removed, but it is still present", i)
			}
		}
		if !strings.Contains(out, roleBodies[i]) {
			t.Errorf("input %d: expected role body preserved, missing", i)
		}
	}
}

func TestStrip_BelowBootstrapThreshold_ReturnsUnchanged(t *testing.T) {
	shared := joinParas(companyBlurbP1, companyBlurbP2)
	inputs := []string{
		shared + "\n\nFirst role body unique text here.",
		shared + "\n\nSecond role body unique text here.",
	}

	got := boilerplate.Strip(inputs)
	if len(got) != len(inputs) {
		t.Fatalf("len mismatch")
	}
	for i := range inputs {
		if got[i] != inputs[i] {
			t.Errorf("input %d: expected byte-for-byte unchanged\n got: %q\nwant: %q", i, got[i], inputs[i])
		}
	}
}

func TestStrip_DivergingSamples_ReturnsUnchanged(t *testing.T) {
	inputs := []string{
		"Alpha role description, fully unique paragraph that does not repeat anywhere.",
		"Beta role description, also fully unique, never recurs in any other input.",
		"Gamma role description with its own particular wording and no shared content.",
		"Delta role posting using novel phrasing not echoed elsewhere in the batch.",
		"Epsilon role posting, again unique to this single sample of the dataset.",
		"Zeta role posting with content found only here and nowhere else at all.",
		"Eta role posting whose paragraphs do not appear in sibling descriptions.",
		"Theta role posting wrapping up the batch with original prose only.",
	}

	got := boilerplate.Strip(inputs)
	for i := range inputs {
		if got[i] != inputs[i] {
			t.Errorf("input %d: expected unchanged\n got: %q\nwant: %q", i, got[i], inputs[i])
		}
	}
}

func TestStrip_EntireDescriptionIsBoilerplate_LeavesUnchanged(t *testing.T) {
	blurb := joinParas(companyBlurbP1, companyBlurbP2, companyBlurbP3, companyBlurbP4)
	inputs := []string{
		blurb + "\n\nUnique role body for engineer one with custom responsibilities and team context goes here.",
		blurb + "\n\nUnique role body for engineer two outlining a different scope and reporting structure.",
		blurb + "\n\nUnique role body for engineer three on a totally different team with separate goals.",
		blurb + "\n\nUnique role body for engineer four covering yet another product area entirely apart.",
		blurb, // entirely boilerplate
	}

	got := boilerplate.Strip(inputs)

	// The all-boilerplate input must be returned unchanged.
	if got[4] != inputs[4] {
		t.Errorf("expected all-boilerplate input unchanged\n got: %q\nwant: %q", got[4], inputs[4])
	}

	// The other four should have the blurb stripped.
	for i := 0; i < 4; i++ {
		for _, p := range []string{companyBlurbP1, companyBlurbP2, companyBlurbP3, companyBlurbP4} {
			if strings.Contains(got[i], p) {
				t.Errorf("input %d: blurb paragraph still present", i)
			}
		}
	}
}

func TestStrip_MultipleDisjointBlocks_StripsBoth(t *testing.T) {
	roleBodies := []string{
		"Senior backend engineer joining the Payments team to modernize our settlement pipeline and refactor the daily reconciliation jobs.",
		"Site reliability engineer focused on the database fleet, partnering with the storage team on Postgres major-version upgrades.",
		"Frontend engineer building the new admin console used by enterprise customer success managers around the world.",
		"Machine learning engineer joining the Search quality team to iterate on ranking models and offline evaluation pipelines.",
		"Security engineer joining the Detection and Response group to expand our SIEM coverage and tune alerting fidelity across cloud accounts.",
	}

	inputs := make([]string, len(roleBodies))
	for i, body := range roleBodies {
		inputs[i] = joinParas(companyBlurbP1, body, eeoFooter)
	}

	got := boilerplate.Strip(inputs)

	for i, out := range got {
		if strings.Contains(out, companyBlurbP1) {
			t.Errorf("input %d: top blurb still present", i)
		}
		if strings.Contains(out, eeoFooter) {
			t.Errorf("input %d: EEO footer still present", i)
		}
		if !strings.Contains(out, roleBodies[i]) {
			t.Errorf("input %d: role body missing", i)
		}
	}
}

func TestStrip_CRLFLineEndings_NormalizesCorrectly(t *testing.T) {
	// joinCRLF joins paragraphs with CRLF paragraph separators (\r\n\r\n)
	// to exercise the normalizeNewlines path inside splitParagraphs.
	joinCRLF := func(paras ...string) string {
		return strings.Join(paras, "\r\n\r\n")
	}

	roleBodies := []string{
		"You will own the ingestion pipeline end to end, covering schema design, backfill orchestration, and the on-call rotation for the three dependent services that rely on real-time data to drive customer-facing features.",
		"This role joins the Platform Security team to harden secrets management across our Kubernetes fleet, drive zero-trust network segmentation, and lead the rollout of hardware-backed identity for service-to-service authentication.",
		"As a senior engineer on the Growth Infrastructure team, you will build and maintain the experimentation platform that runs hundreds of A/B tests per quarter, including the assignment service, the metrics pipeline, and the analysis tooling.",
		"Join the Reliability Engineering group to define and enforce SLOs across all product APIs, build out structured-error budgeting dashboards, and partner with product teams on release-gate policies that keep error rates below contractual thresholds.",
	}

	inputs := make([]string, len(roleBodies))
	for i, body := range roleBodies {
		// Use \r\n\r\n as paragraph separator to simulate Windows-style line endings.
		inputs[i] = joinCRLF(companyBlurbP1, body)
	}

	got := boilerplate.Strip(inputs)
	if len(got) != len(inputs) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(inputs))
	}

	for i, out := range got {
		if strings.Contains(out, companyBlurbP1) {
			t.Errorf("input %d: shared blurb still present after stripping", i)
		}
		if !strings.Contains(out, roleBodies[i]) {
			t.Errorf("input %d: unique role body missing from output", i)
		}
	}
}

func TestStrip_OverlappingBoilerplate_PrefersLongest(t *testing.T) {
	// longBlock appears in all inputs as a standalone paragraph.
	longBlock := "Acme Corporation offers a comprehensive benefits package including full medical, dental, and vision coverage for employees and their dependents, a four-percent employer 401k match with immediate vesting, twelve weeks of paid parental leave for all parents, an annual learning stipend of two thousand dollars, and a monthly remote-work allowance to cover home-office expenses."

	// shortBlock is a strict prefix of longBlock and also >200 chars. It
	// appears in all inputs as a standalone paragraph so it qualifies
	// independently. dropSubstrings should keep only longBlock.
	shortBlock := longBlock[:220]

	// Confirm test invariants about the two blocks.
	if len(longBlock) <= 200 {
		// This is a test-construction error, not a Strip error.
		panic("longBlock must be >200 chars")
	}
	if len(shortBlock) <= 200 {
		panic("shortBlock must be >200 chars")
	}
	if !strings.Contains(longBlock, shortBlock) {
		panic("shortBlock must be a strict substring of longBlock")
	}
	if longBlock == shortBlock {
		panic("longBlock and shortBlock must differ")
	}

	roleBodies := []string{
		"You will lead the mobile release-engineering effort across iOS and Android, owning the build system, code-signing infrastructure, and the phased rollout tooling used by all eight mobile product squads.",
		"As a backend engineer on the Developer Experience team, you will build internal SDKs, maintain the shared service mesh configuration, and run the monthly platform-health review with engineering leadership.",
		"This role joins the Trust and Safety team to design the abuse-detection pipeline, tune ML classifiers, and partner with policy on the escalation workflows that handle high-severity content violations.",
		"Join the Distributed Systems team to architect the next generation of our global scheduling fabric, covering resource allocation, task preemption, and cross-region failover for our highest-priority customer workloads.",
		"Senior engineer on the Analytics Platform, responsible for the real-time aggregation layer, the materialized-view refresh scheduler, and the self-serve SQL environment used by over two hundred internal analysts.",
	}

	inputs := make([]string, len(roleBodies))
	for i, body := range roleBodies {
		// Each input contains both the long and short block as separate paragraphs.
		inputs[i] = joinParas(longBlock, shortBlock, body)
	}

	got := boilerplate.Strip(inputs)
	if len(got) != len(inputs) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(inputs))
	}

	for i, out := range got {
		// longBlock must be stripped — it is the sole qualifying paragraph
		// after dropSubstrings keeps the longest and discards shortBlock.
		if strings.Contains(out, longBlock) {
			t.Errorf("input %d: longBlock still present after stripping", i)
		}
		// shortBlock was dropped from the qualifying set by dropSubstrings
		// (it is a strict substring of longBlock), so Strip does not remove it
		// as an independent paragraph. It may still be present in the output.
		// We only assert it was NOT double-stripped (i.e., not absent due to
		// some cascade effect): if the role body is present, the output is
		// non-empty and structurally sound.
		if !strings.Contains(out, roleBodies[i]) {
			t.Errorf("input %d: unique role body missing from output", i)
		}
	}
}

func TestStrip_OrderPreservation(t *testing.T) {
	roleBodies := []string{
		"FIRST unique role body covering the platform team's quarterly objectives in granular detail across multiple workstreams.",
		"SECOND unique role body about the data platform, with specific call-outs to the streaming and warehousing tracks.",
		"THIRD unique role body talking about developer experience, internal tools, and the shared CI/CD platform team.",
		"FOURTH unique role body for the mobile org, covering both iOS and Android with an emphasis on release engineering.",
		"FIFTH unique role body for the growth team focused on experimentation infrastructure and feature-flag tooling.",
	}
	inputs := make([]string, len(roleBodies))
	for i, body := range roleBodies {
		inputs[i] = joinParas(companyBlurbP1, companyBlurbP2, body)
	}

	got := boilerplate.Strip(inputs)
	if len(got) != len(inputs) {
		t.Fatalf("len mismatch")
	}
	for i, out := range got {
		if !strings.Contains(out, roleBodies[i]) {
			t.Errorf("output index %d does not contain expected role body %q", i, roleBodies[i])
		}
		// And confirm it does NOT contain another input's role body.
		for j, other := range roleBodies {
			if i == j {
				continue
			}
			if strings.Contains(out, other) {
				t.Errorf("output index %d unexpectedly contains role body from index %d", i, j)
			}
		}
	}
}
