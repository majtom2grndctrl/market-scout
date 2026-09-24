package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

// sampleLine is one probe-sample posting. Half splits the sample for the
// description arm: role descriptions are written from half A and scored on
// half B, so no description is scored on a posting it was written from.
type sampleLine struct {
	PostingID         int64  `json:"posting_id"`
	CompanyID         int64  `json:"company_id"`
	CompanyName       string `json:"company_name"`
	Half              string `json:"half"`
	Title             string `json:"title"`
	MaskedDescription string `json:"masked_description"`
}

// labelerLine is what a gold labeler reads: the title-masked description and
// nothing else, so a label cannot come from the title.
type labelerLine struct {
	PostingID         int64  `json:"posting_id"`
	MaskedDescription string `json:"masked_description"`
}

// halfOf splits by posting id parity: stable, and independent of order.
func halfOf(id int64) string {
	if id%2 == 0 {
		return "A"
	}
	return "B"
}

// SampleConfig is the parsed `classify sample` command line.
type SampleConfig struct {
	Count         int
	MaxPerCompany int
	OutDir        string
}

func parseSampleFlags(fs *flag.FlagSet, args []string) (SampleConfig, error) {
	var cfg SampleConfig
	fs.IntVar(&cfg.Count, "count", 150, "postings in the probe sample")
	fs.IntVar(&cfg.MaxPerCompany, "max-per-company", 0, "per-company cap (0 = selection default)")
	fs.StringVar(&cfg.OutDir, "out", "agent-output/classify/probe", "directory for sample.jsonl and labeler-input.jsonl")
	if err := fs.Parse(args); err != nil {
		return SampleConfig{}, err
	}
	if cfg.Count < 1 {
		return SampleConfig{}, fmt.Errorf("--count must be at least 1, got %d", cfg.Count)
	}
	return cfg, nil
}

// runSample selects a fresh probe sample under the selection core's
// per-company cap and writes it. It reads only and sends nothing to Jev.
func runSample(args []string) int {
	cfg, err := parseSampleFlags(flag.NewFlagSet("classify sample", flag.ContinueOnError), args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] startup error: %v\n", err)
		return 2
	}
	ctx, stop, pool, code := openReadOnly()
	if code != 0 {
		return code
	}
	defer stop()
	defer pool.Close()

	selected, _, err := selection.Select(ctx, pool, selection.Criteria{Count: cfg.Count, MaxPerCompany: cfg.MaxPerCompany})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] select: %v\n", err)
		return 1
	}
	postings, failed, err := cleanByCompany(ctx, dbCleaner(db.New(pool)), selected)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
		return 1
	}
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "[classify] %d postings could not be cleaned and are left out of the sample\n", len(failed))
	}
	names := map[int64]string{}
	for _, p := range selected {
		names[p.CompanyID] = p.CompanyName
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[classify] create output dir: %v\n", err)
		return 1
	}
	var full, labeler []any
	for _, p := range postings {
		masked := maskTitle(p.Description, p.Title)
		full = append(full, sampleLine{PostingID: p.ID, CompanyID: p.CompanyID, CompanyName: names[p.CompanyID],
			Half: halfOf(p.ID), Title: p.Title, MaskedDescription: masked})
		labeler = append(labeler, labelerLine{PostingID: p.ID, MaskedDescription: masked})
	}
	for name, lines := range map[string][]any{"sample.jsonl": full, "labeler-input.jsonl": labeler} {
		if err := writeJSONLFile(filepath.Join(cfg.OutDir, name), lines); err != nil {
			fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "[classify] wrote %d postings to %s\n", len(postings), cfg.OutDir)
	return 0
}

// readSampleIDs reads the posting ids from a sample file.
func readSampleIDs(path string) ([]int64, error) {
	lines, err := readJSONL[sampleLine](path)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("sample %s holds no postings", path)
	}
	ids := make([]int64, len(lines))
	for i, l := range lines {
		ids[i] = l.PostingID
	}
	return ids, nil
}

// postingsByID adapts ListPostingsForClassify to selection's shape.
func postingsByID(ctx context.Context, q *db.Queries, ids []int64) ([]selection.Posting, error) {
	rows, err := q.ListPostingsForClassify(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, fmt.Errorf("sample names %d postings, %d have a description", len(ids), len(rows))
	}
	out := make([]selection.Posting, len(rows))
	for i, r := range rows {
		out[i] = selection.Posting{PostingID: r.PostingID, CompanyID: r.CompanyID, CompanyName: r.CompanyName,
			Title: r.Title, DescriptionText: r.DescriptionText, IsRepresentative: true}
	}
	return out, nil
}

func writeJSONLFile(path string, lines []any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	w := &jsonlWriter{w: f}
	for _, l := range lines {
		if err := w.write(l); err != nil {
			return err
		}
	}
	return f.Close()
}

// readJSONL decodes every line of a JSONL file as T.
func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	var out []T
	r := bufio.NewReader(f)
	for n := 1; ; n++ {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && string(line) != "\n" {
			var v T
			if jerr := json.Unmarshal(line, &v); jerr != nil {
				return nil, fmt.Errorf("%s line %d: %w", path, n, jerr)
			}
			out = append(out, v)
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
	}
}

// readRoleDescriptions reads the description arm's input: a JSON object of
// role slug to description.
func readRoleDescriptions(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading role descriptions: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decoding role descriptions %s: %w", path, err)
	}
	return m, nil
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// readJSONArray decodes a file holding one JSON array of T.
func readJSONArray[T any](path string) ([]T, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var out []T
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return out, nil
}
