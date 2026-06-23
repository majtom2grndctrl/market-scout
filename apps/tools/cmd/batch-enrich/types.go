// Shared contract types for the batch-enrich package: postings pulled from
// the database for classification, and the in-memory taxonomy snapshot the
// orchestrator hands to every agent prompt.
//
// The classifier payload contract (AgentResponse and friends) and the taxonomy
// snapshot live in internal/enrich/classify, shared with the MCP
// save_enrichment action. They are aliased here so batch-enrich code keeps its
// existing names while there is a single source of truth for the contract.
package main

import "github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"

// SelectedPosting is a posting chosen for this run, paired with the latest
// snapshot's title and description text. The description may be the raw
// snapshot text or the boilerplate-stripped variant; see boilerplate.go
// for stripping threshold and logic.
type SelectedPosting struct {
	PostingID       int64
	CompanyID       int64
	Title           string
	DescriptionText string
}

// TaxonomyEntry and Taxonomy alias the shared classify types so the
// orchestrator, prompt rendering, validation, and writeback continue to refer
// to them by their original names. LoadTaxonomy (select.go) builds the value.
type TaxonomyEntry = classify.TaxonomyEntry
type Taxonomy = classify.Taxonomy

// AgentResponse and its nested shapes alias the shared classifier contract.
// The Haiku classifier emits this JSON per posting; the MCP save_enrichment
// action accepts the same shape from an agent.
type AgentResponse = classify.AgentResponse
type AgentClassification = classify.AgentClassification
type AgentCanonicalRole = classify.AgentCanonicalRole
type AgentSpecialization = classify.AgentSpecialization
type AgentSkill = classify.AgentSkill
