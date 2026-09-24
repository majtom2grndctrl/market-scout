package main

// Question wording. Every string here is part of the classify contract: a
// change bumps PromptVersion, and the run's question hash records it.

// noneFit is the abstain option offered beside every role set. Jev's choice has
// no built-in abstain.
const noneFit = "none_fit"

var noneFitDescription = ptr("None of the listed roles fits the job this posting hires for.")

const (
	rolePass1Instructions = "The job title has been removed from this job posting. " +
		"Judging from the duties and requirements it describes, which of these job roles is it hiring for?"
	rolePass2Instructions = rolePass1Instructions
)

// maskPlaceholder replaces a masked title. It holds no letters, so it can
// never re-form a title or collide with one.
const maskPlaceholder = "[—]"

func ptr(s string) *string { return &s }
