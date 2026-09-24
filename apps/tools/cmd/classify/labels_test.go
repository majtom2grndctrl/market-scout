package main

import (
	"context"
	"slices"
	"testing"
)

func jevLabel(slug string, p float64) label {
	return label{Slug: slug, Name: slug, Method: methodJev, Probability: p}
}

func slugs(ls []label) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Slug
	}
	return out
}

func TestLabelRule(t *testing.T) {
	rule := labelRule{Floor: 0.5, Relative: 0.5, Cap: 3}
	cases := []struct {
		name  string
		cands []label
		want  []string
	}{
		{"a label at the floor is kept", []label{jevLabel("a", 0.5)}, []string{"a"}},
		{"a label just below the floor is dropped", []label{jevLabel("a", 0.4999)}, nil},
		{"a label at half the top is kept", []label{jevLabel("top", 1.0), jevLabel("half", 0.5)}, []string{"top", "half"}},
		{"a label just below half the top is dropped", []label{jevLabel("top", 1.0), jevLabel("under", 0.4999)}, []string{"top"}},
		{"half is relative to the top, not the floor",
			[]label{jevLabel("top", 0.9), jevLabel("mid", 0.6), jevLabel("low", 0.44)}, []string{"top", "mid"}},
		{"past the cap the least probable go first",
			[]label{jevLabel("d", 0.6), jevLabel("a", 0.9), jevLabel("c", 0.7), jevLabel("b", 0.8)}, []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := slugs(rule.apply(tc.cands)); !slices.Equal(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Errorf("kept %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLabelRule_ZeroSurvivors(t *testing.T) {
	if got := pinnedThresholds.Specializations.apply([]label{jevLabel("a", 0.1), jevLabel("b", 0.2)}); len(got) != 0 {
		t.Errorf("kept %v, want none", slugs(got))
	}
	if got := pinnedThresholds.Specializations.apply(nil); len(got) != 0 {
		t.Errorf("kept %v from no candidates", slugs(got))
	}
}

func TestRecordingFloor(t *testing.T) {
	got := recorded([]label{jevLabel("at", 0.05), jevLabel("below", 0.0499), jevLabel("above", 0.9)}, 0.05)
	if !slices.Equal(slugs(got), []string{"at", "above"}) {
		t.Errorf("recorded %v, want [at above]", slugs(got))
	}
}

// pairFinder reports fixed near-duplicate pairs by slug.
type pairFinder struct {
	pairs [][2]string
	calls int
}

func (f *pairFinder) nearDuplicatePairs(_ context.Context, terms []label) ([][2]int, error) {
	f.calls++
	idx := map[string]int{}
	for i, t := range terms {
		idx[t.Slug] = i
	}
	var out [][2]int
	for _, p := range f.pairs {
		a, okA := idx[p[0]]
		b, okB := idx[p[1]]
		if okA && okB {
			out = append(out, [2]int{a, b})
		}
	}
	return out, nil
}

func TestNearDuplicatePredup(t *testing.T) {
	t.Run("the more probable side of a pair is kept", func(t *testing.T) {
		f := &pairFinder{pairs: [][2]string{{"customer-success", "customer-sucess"}}}
		got, err := dropNearDuplicates(t.Context(), f, []label{
			jevLabel("customer-sucess", 0.9), jevLabel("customer-success", 0.7), jevLabel("renewals", 0.6),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(slugs(got), []string{"customer-sucess", "renewals"}) {
			t.Errorf("kept %v", slugs(got))
		}
	})

	t.Run("quotable evidence beats a model judgment", func(t *testing.T) {
		f := &pairFinder{pairs: [][2]string{{"kubernetes", "kubernetes-ops"}}}
		lex := label{Slug: "kubernetes", Name: "Kubernetes", Method: methodLexical}
		got, _ := dropNearDuplicates(t.Context(), f, []label{jevLabel("kubernetes-ops", 0.99), lex})
		if !slices.Equal(slugs(got), []string{"kubernetes"}) {
			t.Errorf("kept %v", slugs(got))
		}
	})

	t.Run("a chain keeps both ends when they are not a pair themselves", func(t *testing.T) {
		// Greedy over preference: a is kept, b pairs with a and goes, c pairs
		// only with b, which is gone, so c stays. The save's pass does the same.
		f := &pairFinder{pairs: [][2]string{{"a", "b"}, {"b", "c"}}}
		got, _ := dropNearDuplicates(t.Context(), f, []label{jevLabel("a", 0.9), jevLabel("b", 0.8), jevLabel("c", 0.7)})
		if !slices.Equal(slugs(got), []string{"a", "c"}) {
			t.Errorf("kept %v", slugs(got))
		}
	})

	t.Run("fewer than two labels skips the lookup", func(t *testing.T) {
		f := &pairFinder{}
		_, _ = dropNearDuplicates(t.Context(), f, []label{jevLabel("a", 0.9)})
		if f.calls != 0 {
			t.Error("queried the database for a single label")
		}
	})
}
