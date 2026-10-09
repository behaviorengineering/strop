package jev

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultMaxQuestions is the TypeSafe SystemOne cap per request.
const DefaultMaxQuestions = 64

// Question is one typed question on POST /v1/systemone.
type Question struct {
	Type         string
	Instructions string
	Criteria     map[string]string
}

// Request is one SystemOne eval call.
type Request struct {
	Model     string
	State     string
	Questions map[string]Question
}

// Answer is one answer in a SystemOne response.
type Answer struct {
	Type   string
	Noul   float64
	Choice []byte
}

// Row is one logical row to pack into SystemOne requests.
type Row struct {
	ID          string
	StateSuffix string
	Questions   map[string]Question
}

// BatchRows groups rows by identical option-id sets and chunks by maxQuestions.
func BatchRows(rows []Row, world string, maxQuestions int) ([]Request, error) {
	if maxQuestions <= 0 {
		maxQuestions = DefaultMaxQuestions
	}
	world = strings.TrimSpace(world)

	filtered := make([]Row, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ID) == "" {
			continue
		}
		filtered = append(filtered, row)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	groups := map[string][]Row{}
	groupOpts := map[string][]string{}
	for _, row := range filtered {
		opts := optionIDs(row.Questions)
		if len(opts) == 0 {
			return nil, fmt.Errorf("jev: batch rows: row %q has no questions", row.ID)
		}
		if len(opts) > maxQuestions {
			return nil, fmt.Errorf("jev: batch rows: row %q has %d questions (max %d)", row.ID, len(opts), maxQuestions)
		}
		key := optionSetKey(opts)
		groups[key] = append(groups[key], row)
		groupOpts[key] = opts
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Request
	for _, gk := range keys {
		opts := groupOpts[gk]
		grp := groups[gk]
		sort.Slice(grp, func(i, j int) bool {
			return grp[i].ID < grp[j].ID
		})
		nOpt := len(opts)
		chunkSize := maxQuestions / nOpt
		if chunkSize < 1 {
			return nil, fmt.Errorf("jev: batch rows: option set too wide for max %d", maxQuestions)
		}
		for start := 0; start < len(grp); start += chunkSize {
			end := start + chunkSize
			if end > len(grp) {
				end = len(grp)
			}
			chunk := grp[start:end]
			req, err := buildRequest(world, chunk, opts)
			if err != nil {
				return nil, err
			}
			out = append(out, req)
		}
	}
	return out, nil
}

func optionIDs(q map[string]Question) []string {
	ids := make([]string, 0, len(q))
	for id := range q {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func buildRequest(world string, chunk []Row, opts []string) (Request, error) {
	var state strings.Builder
	if world != "" {
		state.WriteString(world)
	}
	questions := make(map[string]Question)
	for _, row := range chunk {
		if state.Len() > 0 {
			state.WriteString("\n\n")
		}
		state.WriteString(row.ID)
		state.WriteString(":\n")
		state.WriteString(strings.TrimSpace(row.StateSuffix))
		for _, opt := range opts {
			q, ok := row.Questions[opt]
			if !ok {
				return Request{}, fmt.Errorf("jev: batch rows: row %q missing option %q", row.ID, opt)
			}
			if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(opt) == "" {
				return Request{}, fmt.Errorf("jev: batch rows: empty row id or option id")
			}
			questions[JoinKey(row.ID, opt)] = q
		}
	}
	return Request{
		State:     strings.TrimSpace(state.String()),
		Questions: questions,
	}, nil
}
