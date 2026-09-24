package main

import "fmt"

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

// Noul wording for the multi-label dimensions. Each term is asked by name.
func specInstructions(name string) string {
	return fmt.Sprintf("Does the job this posting hires for work in the area of %s?", name)
}

func skillInstructions(name string) string {
	return fmt.Sprintf("Does this job posting ask the candidate for %s?", name)
}

// seniorityInstructions asks whether one level phrase, read in its sentence,
// states the level of the job. v9's Step 1: the word counts only when it acts
// as a level ("Senior Engineer"), not when it describes something else
// ("partner with senior stakeholders", "Chief of Staff").
func seniorityInstructions(phrase, context string) string {
	return fmt.Sprintf("In the passage %q, does %q state the seniority level of the job this posting is hiring for?", context, phrase)
}

// seniorityState is what the seniority questions read: the title, then the
// cleaned description.
func seniorityState(p posting) string {
	return "Job title: " + p.Title + "\n\n" + p.Description
}

// maskPlaceholder replaces a masked title. It holds no letters, so it can
// never re-form a title or collide with one.
const maskPlaceholder = "[—]"

func ptr(s string) *string { return &s }
