package atsdetect

import "strings"

// NormalizeBoardToken canonicalizes a board token per ATS so the same company
// board cannot enter the system twice under different casing. It is the
// single source of truth for this rule — detection (detect.go) and the
// add_company write boundary both route through it rather than keeping their
// own copy.
//
// Rules, derived from live API probes (not assumption):
//
//   - greenhouse: lowercase. `boards-api.greenhouse.io/v1/boards/{stripe,Stripe,STRIPE}/jobs`
//     all returned HTTP 200 — the API is case-insensitive, so canonicalizing
//     to lowercase is safe and collapses casing variants of the same board.
//   - ashby: lowercase. `api.ashbyhq.com/posting-api/job-board/{qawolf,QAWolf}`
//     both returned HTTP 200 with the same two jobs; `{MotherDuck,motherduck}`
//     both returned 200 too.
//   - workday: lowercase only the host segment (before the first "/"). The
//     token is `{host}/{site}`; host is a DNS name (case-insensitive), but
//     site is an opaque tenant path segment Workday has not been probed as
//     case-insensitive for, so it is left untouched.
//   - workable: lowercase. The slug is already documented and enforced as
//     lowercase elsewhere (see ValidateBoardToken); this makes the rule
//     consistent with that at the normalization boundary too.
//   - lever and gem: returned UNCHANGED. Greenhouse, Ashby, and Workable
//     lowercase their full tokens; Workday lowercases only its DNS host. This
//     distinction is deliberate: `api.lever.co/v0/postings/Mistral` -> 404,
//     `api.lever.co/v0/postings/mastreforestation` -> 404, and
//     `api.lever.co/v0/postings/ridwell` -> 404, but the correctly-cased
//     `MastReforestation` -> 200 and `Ridwell` -> 200. Lever's API is
//     case-sensitive. Lowercasing a Lever token breaks a live board. Do not
//     "fix" this to match the other ATS platforms. Gem is also case-sensitive:
//     `supio` returned 200 while `Supio` and `SUPIO` returned 404 from
//     `api.gem.com/job_board/v0/{token}/job_posts/`.
//
// Unknown ats values return token unchanged — normalization is opt-in per
// known ATS, not a blanket transform.
func NormalizeBoardToken(ats, token string) string {
	switch ats {
	case "greenhouse", "ashby", "workable":
		return strings.ToLower(token)
	case "workday":
		host, rest, found := strings.Cut(token, "/")
		if !found {
			return strings.ToLower(host)
		}
		return strings.ToLower(host) + "/" + rest
	case "lever", "gem":
		return token
	default:
		return token
	}
}
