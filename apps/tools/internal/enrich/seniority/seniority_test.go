package seniority

import (
	"reflect"
	"strings"
	"testing"
)

const floor = 0.5

// judge is the fake Jev: each phrase in yes is judged a level at the given
// probability, and every other candidate is judged not one.
func judge(cands []Candidate, yes map[string]float64) []float64 {
	probs := make([]float64, len(cands))
	for i, c := range cands {
		probs[i] = yes[c.Phrase]
		if p, ok := yes[string(c.Location)+":"+c.Phrase]; ok {
			probs[i] = p
		}
	}
	return probs
}

type want struct {
	value     string
	note      string
	uncovered bool
}

func runCases(t *testing.T, cases []struct {
	name        string
	title, body string
	yes         map[string]float64
	want        want
}) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cands := Extract(tc.title, tc.body)
			got := Resolve(cands, judge(cands, tc.yes), floor)
			if got.Value != tc.want.value || got.Note() != tc.want.note || got.Uncovered != tc.want.uncovered {
				t.Errorf("got value %q note %q uncovered %v, want value %q note %q uncovered %v\ncandidates: %+v",
					got.Value, got.Note(), got.Uncovered, tc.want.value, tc.want.note, tc.want.uncovered, cands)
			}
		})
	}
}

// TestSeniority_V9Examples is acceptance row S1: v9's Step 1 examples against
// a fake judge.
func TestSeniority_V9Examples(t *testing.T) {
	runCases(t, []struct {
		name        string
		title, body string
		yes         map[string]float64
		want        want
	}{
		{
			name:  "level word in the title",
			title: "Staff Software Engineer",
			yes:   map[string]float64{"Staff": 0.9},
			want:  want{value: Staff, note: `seniority[step1-title]: "Staff"`},
		},
		{
			name:  "level word in the body",
			title: "Software Engineer",
			body:  "We are hiring. This is a mid-level role on the payments team.",
			yes:   map[string]float64{"mid-level": 0.9},
			want:  want{value: Mid, note: `seniority[step1-body]: "mid-level"`},
		},
		{
			name:  "senior stakeholders is not a level",
			title: "Account Executive",
			body:  "You will present to senior stakeholders across the business.",
			yes:   map[string]float64{"senior": 0.1},
			want:  want{value: Unknown},
		},
		{
			name:  "Chief of Staff carries no level",
			title: "Chief of Staff",
			yes:   map[string]float64{"Staff": 0.05},
			want:  want{value: Unknown},
		},
		{
			name:  "no candidate judged a level is unknown with no note",
			title: "Software Engineer II",
			body:  "You will lead projects and partner with our Director of Engineering.",
			want:  want{value: Unknown},
		},
		{
			name:  "no candidate at all is unknown with no note",
			title: "Product Designer",
			body:  "Design delightful things.",
			want:  want{value: Unknown},
		},
		{
			name:  "compound senior staff",
			title: "Senior Staff Software Engineer",
			yes:   map[string]float64{"Senior Staff": 0.9},
			want:  want{value: Staff, note: `seniority[step1-title]: "Senior Staff"`},
		},
		{
			name:  "compound Sr. Staff",
			title: "Sr. Staff Engineer",
			yes:   map[string]float64{"Sr. Staff": 0.9},
			want:  want{value: Staff, note: `seniority[step1-title]: "Sr. Staff"`},
		},
		{
			name:  "compound senior-staff",
			title: "senior-staff engineer",
			yes:   map[string]float64{"senior-staff": 0.9},
			want:  want{value: Staff, note: `seniority[step1-title]: "senior-staff"`},
		},
		{
			name:  "Head of Design is director",
			title: "Head of Design",
			yes:   map[string]float64{"Head of Design": 0.9},
			want:  want{value: Director, note: `seniority[step1-title]: "Head of Design"`},
		},
		{
			name:  "VP of Engineering is director",
			title: "VP of Engineering, Platform",
			yes:   map[string]float64{"VP of Engineering": 0.9},
			want:  want{value: Director, note: `seniority[step1-title]: "VP of Engineering"`},
		},
		{
			name:  "General Manager is director",
			title: "General Manager (Italy)",
			yes:   map[string]float64{"General Manager": 0.9},
			want:  want{value: Director, note: `seniority[step1-title]: "General Manager"`},
		},
		{
			name:  "Associate Director is director",
			title: "Associate Director, Finance",
			yes:   map[string]float64{"Associate Director": 0.9},
			want:  want{value: Director, note: `seniority[step1-title]: "Associate Director"`},
		},
		{
			name:  "Senior Manager is senior",
			title: "Senior Manager, Customer Success",
			yes:   map[string]float64{"Senior Manager": 0.9},
			want:  want{value: Senior, note: `seniority[step1-title]: "Senior Manager"`},
		},
		{
			name:  "Senior Director is director",
			title: "Senior Director of Sales",
			yes:   map[string]float64{"Senior Director": 0.9},
			want:  want{value: Director, note: `seniority[step1-title]: "Senior Director"`},
		},
		{
			name:  "Senior Lead is lead",
			title: "Senior Lead Designer",
			yes:   map[string]float64{"Senior Lead": 0.9},
			want:  want{value: Lead, note: `seniority[step1-title]: "Senior Lead"`},
		},
		{
			name:  "Senior Principal Architect is principal",
			title: "Senior Principal Architect",
			yes:   map[string]float64{"Senior Principal Architect": 0.9},
			want:  want{value: Principal, note: `seniority[step1-title]: "Senior Principal Architect"`},
		},
		{
			name:  "band Senior/Staff takes the lower bound",
			title: "Senior/Staff Engineer",
			yes:   map[string]float64{"Senior/Staff": 0.9},
			want:  want{value: Senior, note: `seniority[step1-title]: "Senior/Staff"`},
		},
		{
			name:  "band Staff + Sr. takes the lower bound",
			title: "Staff + Sr. Engineer",
			yes:   map[string]float64{"Staff + Sr.": 0.9},
			want:  want{value: Senior, note: `seniority[step1-title]: "Staff + Sr."`},
		},
		{
			name:  "band Mid-Senior takes the lower bound",
			title: "Mid-Senior Account Executive",
			yes:   map[string]float64{"Mid-Senior": 0.9},
			want:  want{value: Mid, note: `seniority[step1-title]: "Mid-Senior"`},
		},
		{
			name:  "band Entry-level to Mid takes the lower bound",
			title: "Analyst",
			body:  "This role is Entry-level to Mid depending on experience.",
			yes:   map[string]float64{"Entry-level to Mid": 0.9},
			want:  want{value: Junior, note: `seniority[step1-body]: "Entry-level to Mid"`},
		},
		{
			name:  "band senior or lead takes the lower bound",
			title: "Designer",
			body:  "We will hire at senior or lead level.",
			yes:   map[string]float64{"senior or lead": 0.9},
			want:  want{value: Senior, note: `seniority[step1-body]: "senior or lead"`},
		},
		{
			name:  "bare Manager is never a level",
			title: "Engineering Manager",
			body:  "As a Manager you will run the team.",
			want:  want{value: Unknown},
		},
		{
			name:  "adjacent pair missing from the table is uncovered",
			title: "Principal Staff Engineer",
			yes:   map[string]float64{"Principal Staff": 0.9},
			want:  want{value: Unknown, uncovered: true},
		},
		{
			name:  "band with an uncovered member is uncovered",
			title: "Senior VP / Director of Sales",
			yes:   map[string]float64{"Senior VP / Director": 0.9},
			want:  want{value: Unknown, uncovered: true},
		},
		{
			name:  "title beats a more probable body candidate",
			title: "Senior Product Designer",
			body:  "This is a Principal-track role.",
			yes:   map[string]float64{"Senior": 0.6, "Principal": 0.99},
			want:  want{value: Senior, note: `seniority[step1-title]: "Senior"`},
		},
		{
			name:  "most probable body candidate wins",
			title: "Product Designer",
			body:  "You will mentor junior designers. This is a staff-level role.",
			yes:   map[string]float64{"junior": 0.6, "staff": 0.95},
			want:  want{value: Staff, note: `seniority[step1-body]: "staff"`},
		},
		{
			name:  "body tie goes to the first occurrence",
			title: "Product Designer",
			body:  "This is a Senior role. It may grow into a Principal role.",
			yes:   map[string]float64{"Senior": 0.8, "Principal": 0.8},
			want:  want{value: Senior, note: `seniority[step1-body]: "Senior"`},
		},
		{
			name:  "title word glued to a stripped heading still counts in the body",
			title: "Product Designer",
			body:  "About the RoleSenior Product Designer on the growth team.",
			yes:   map[string]float64{"Senior": 0.9},
			want:  want{value: Senior, note: `seniority[step1-body]: "Senior"`},
		},
	})
}

// TestSeniority_Judgment is acceptance row S2: the judgment alone decides
// whether a phrase yields seniority.
func TestSeniority_Judgment(t *testing.T) {
	const title = "Lead Data Engineer"
	const body = "You will lead a team of five engineers."
	runCases(t, []struct {
		name        string
		title, body string
		yes         map[string]float64
		want        want
	}{
		{
			name:  "judged not a level gives nothing",
			title: title, body: body,
			yes:  map[string]float64{"title:Lead": 0.2, "body:lead": 0.3},
			want: want{value: Unknown},
		},
		{
			name:  "judged a level gives the v9 value and a verbatim note",
			title: title, body: body,
			yes:  map[string]float64{"title:Lead": 0.85, "body:lead": 0.3},
			want: want{value: Lead, note: `seniority[step1-title]: "Lead"`},
		},
		{
			name:  "exactly at the floor counts",
			title: title, body: body,
			yes:  map[string]float64{"title:Lead": floor},
			want: want{value: Lead, note: `seniority[step1-title]: "Lead"`},
		},
		{
			name:  "body phrase judged a level quotes the body verbatim",
			title: "Data Engineer",
			body:  "We're hiring a Principal Data Center Architect.",
			yes:   map[string]float64{"Principal": 0.9},
			want:  want{value: Principal, note: `seniority[step1-body]: "Principal"`},
		},
	})
}

func TestResolve_MissingProbabilityDoesNotCount(t *testing.T) {
	cands := Extract("Senior Engineer", "")
	if got := Resolve(cands, nil, floor); got.Value != Unknown || got.Note() != "" {
		t.Errorf("Resolve with no probabilities = %+v, want unknown", got)
	}
}

func TestExtract_NoCandidateInsideWords(t *testing.T) {
	for _, text := range []string{
		"Staffing Coordinator",
		"Internal Tools Engineer",
		"Leadership Development Partner",
		"Based at our Headquarters in the Midwest",
		"a misleader amid the headcount-free INTERNAL SENIORITY review",
		"Product Manager",
	} {
		if got := Extract(text, text); len(got) != 0 {
			t.Errorf("Extract(%q) = %+v, want no candidates", text, got)
		}
	}
}

func TestExtract_Candidates(t *testing.T) {
	type c struct{ phrase, value string }
	for _, tc := range []struct {
		name  string
		title string
		want  []c
	}{
		{"Chief of Staff yields a staff candidate for the judge", "Chief of Staff", []c{{"Staff", Staff}}},
		{"Lead Architect is lead, the architect row needs principal", "Lead Architect", []c{{"Lead", Lead}}},
		{"Early career and new grad resolve to junior", "Early Career / New Grad Engineer", []c{{"Early Career / New Grad", Junior}}},
		{"Jr. is junior", "Jr. Analyst", []c{{"Jr.", Junior}}},
		{"internship is intern", "Summer Internship, Design", []c{{"Internship", Intern}}},
		{"separate words stay separate", "Senior Engineer, Platform Lead", []c{{"Senior", Senior}, {"Lead", Lead}}},
		{"three adjacent level words are uncovered", "Senior Staff Principal Engineer", []c{{"Senior Staff Principal", ""}}},
		{"Senior Vice President is uncovered", "Senior Vice President of Sales", []c{{"Senior Vice President of Sales", ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []c
			for _, cand := range Extract(tc.title, "") {
				got = append(got, c{cand.Phrase, cand.Value})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Extract(%q) = %+v, want %+v", tc.title, got, tc.want)
			}
		})
	}
}

func TestExtract_OrderLocationOffsetAndContext(t *testing.T) {
	body := "Our team ships fast. You will mentor junior engineers daily! This is a Staff role."
	got := Extract("Senior Engineer", body)
	want := []Candidate{
		{Phrase: "Senior", Context: "Senior Engineer", Location: LocationTitle, Offset: 0, Value: Senior},
		{Phrase: "junior", Context: "You will mentor junior engineers daily!", Location: LocationBody, Offset: strings.Index(body, "junior"), Value: Junior},
		{Phrase: "Staff", Context: "This is a Staff role.", Location: LocationBody, Offset: strings.Index(body, "Staff"), Value: Staff},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %+v\nwant %+v", got, want)
	}
	if again := Extract("Senior Engineer", body); !reflect.DeepEqual(again, got) {
		t.Errorf("Extract is not deterministic: %+v then %+v", got, again)
	}
}

func TestExtract_ContextIsCappedAroundThePhrase(t *testing.T) {
	long := strings.Repeat("word ", 100)
	body := long + "a senior role " + long
	got := Extract("", body)
	if len(got) != 1 {
		t.Fatalf("Extract = %+v, want one candidate", got)
	}
	ctx := got[0].Context
	if len(ctx) > contextCap || !strings.Contains(ctx, "senior") || !strings.Contains(body, ctx) {
		t.Errorf("context %q (len %d) is not a verbatim window of at most %d bytes around the phrase", ctx, len(ctx), contextCap)
	}
}
