package atsdetect

import "testing"

func TestNormalizeBoardToken(t *testing.T) {
	tests := []struct {
		name  string
		ats   string
		token string
		want  string
	}{
		{name: "greenhouse lowercases", ats: "greenhouse", token: "Stripe", want: "stripe"},
		{name: "greenhouse already lowercase", ats: "greenhouse", token: "stripe", want: "stripe"},
		{name: "ashby lowercases", ats: "ashby", token: "QAWolf", want: "qawolf"},
		{name: "ashby motherduck casing", ats: "ashby", token: "MotherDuck", want: "motherduck"},
		{name: "workable lowercases", ats: "workable", token: "AcmeRobotics", want: "acmerobotics"},
		{
			name:  "workday lowercases host only, site untouched",
			ats:   "workday",
			token: "Acme.WD5.MyWorkdayJobs.com/AcmeCareers",
			want:  "acme.wd5.myworkdayjobs.com/AcmeCareers",
		},
		{
			name:  "workday already lowercase host",
			ats:   "workday",
			token: "acme.wd5.myworkdayjobs.com/AcmeCareers",
			want:  "acme.wd5.myworkdayjobs.com/AcmeCareers",
		},
		{
			name:  "workday token with no site segment lowercases whole token",
			ats:   "workday",
			token: "Acme.WD5.MyWorkdayJobs.com",
			want:  "acme.wd5.myworkdayjobs.com",
		},
		{
			name:  "lever passes through unchanged despite mixed case",
			ats:   "lever",
			token: "MastReforestation",
			want:  "MastReforestation",
		},
		{name: "lever already-lowercase token also unchanged", ats: "lever", token: "ridwell", want: "ridwell"},
		{name: "gem passes through unchanged despite mixed case", ats: "gem", token: "Supio", want: "Supio"},
		{name: "unknown ats returns token unchanged", ats: "rippling", token: "AcmeCo", want: "AcmeCo"},
		{name: "empty ats returns token unchanged", ats: "", token: "AcmeCo", want: "AcmeCo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeBoardToken(tc.ats, tc.token)
			if got != tc.want {
				t.Errorf("NormalizeBoardToken(%q, %q) = %q, want %q", tc.ats, tc.token, got, tc.want)
			}
		})
	}
}
