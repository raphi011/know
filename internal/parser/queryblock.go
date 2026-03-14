package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// QueryFormat indicates how results should be rendered.
type QueryFormat int

const (
	FormatList  QueryFormat = iota // bullet list of links
	FormatTable                    // columnar table
)

// ConditionOp is a WHERE condition operator.
type ConditionOp int

const (
	OpContain  ConditionOp = iota // labels CONTAIN "x" (label membership)
	OpEqual                       // field = "x" (exact match)
	OpContains                    // field CONTAINS "x" (substring)
)

// Condition represents a parsed WHERE clause.
type Condition struct {
	Field string
	Op    ConditionOp
	Value string
}

// QueryBlock represents a parsed know query block.
type QueryBlock struct {
	Index      int         // byte offset of opening ``` in content
	RawQuery   string      // raw DSL text inside the fences
	Folder     *string     // FROM folder
	Conditions []Condition // WHERE clauses
	ShowFields []string    // SHOW columns
	SortField  string      // SORT field
	SortDesc   bool        // SORT DESC
	Limit      int         // LIMIT
	Format     QueryFormat // derived from ShowFields count
	Error      string      // parse error, empty if ok
}

var (
	knowBlockRegex = regexp.MustCompile("(?s)```know\\n(.*?)```")
	whereRegex        = regexp.MustCompile(`(?i)^WHERE\s+(.+)$`)
	fromRegex         = regexp.MustCompile(`(?i)^FROM\s+(\S+)$`)
	showRegex         = regexp.MustCompile(`(?i)^SHOW\s+(.+)$`)
	sortRegex         = regexp.MustCompile(`(?i)^SORT\s+(\S+)(?:\s+(ASC|DESC))?$`)
	limitRegex        = regexp.MustCompile(`(?i)^LIMIT\s+(\d+)$`)

	// WHERE condition patterns
	condContainRegex  = regexp.MustCompile(`(?i)^(\w+)\s+CONTAIN\s+"([^"]+)"$`)
	condEqualRegex    = regexp.MustCompile(`(?i)^(\w+)\s*=\s*"([^"]+)"$`)
	condContainsRegex = regexp.MustCompile(`(?i)^(\w+)\s+CONTAINS\s+"([^"]+)"$`)
)

// ExtractQueryBlocks finds all ```know blocks in content and parses them.
func ExtractQueryBlocks(content string) []QueryBlock {
	matches := knowBlockRegex.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}

	blocks := make([]QueryBlock, 0, len(matches))
	for _, loc := range matches {
		rawQuery := content[loc[2]:loc[3]]
		block := parseQueryBlock(rawQuery)
		block.Index = loc[0]
		block.RawQuery = rawQuery
		blocks = append(blocks, block)
	}
	return blocks
}

func parseQueryBlock(raw string) QueryBlock {
	block := QueryBlock{
		ShowFields: []string{"title", "path"},
		SortField:  "title",
		SortDesc:   false,
		Limit:      50,
		Format:     FormatList,
	}

	hasShow := false
	hasValidLine := false
	lines := strings.SplitSeq(strings.TrimSpace(raw), "\n")

	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if m := fromRegex.FindStringSubmatch(line); m != nil {
			folder := m[1]
			block.Folder = &folder
			hasValidLine = true
		} else if m := whereRegex.FindStringSubmatch(line); m != nil {
			cond, err := parseCondition(m[1])
			if err != nil {
				block.Error = fmt.Sprintf("invalid WHERE: %s", err)
				return block
			}
			block.Conditions = append(block.Conditions, cond)
			hasValidLine = true
		} else if m := showRegex.FindStringSubmatch(line); m != nil {
			fields := strings.Split(m[1], ",")
			block.ShowFields = make([]string, 0, len(fields))
			for _, f := range fields {
				block.ShowFields = append(block.ShowFields, strings.TrimSpace(f))
			}
			hasShow = true
			hasValidLine = true
		} else if m := sortRegex.FindStringSubmatch(line); m != nil {
			block.SortField = strings.ToLower(m[1])
			block.SortDesc = strings.EqualFold(m[2], "DESC")
			hasValidLine = true
		} else if m := limitRegex.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err == nil && n > 0 {
				block.Limit = n
			}
			hasValidLine = true
		} else {
			block.Error = fmt.Sprintf("unrecognized line: %s", line)
			return block
		}
	}

	if !hasValidLine {
		block.Error = "empty query block"
		return block
	}

	// Format detection: >2 SHOW fields = table
	if hasShow && len(block.ShowFields) > 2 {
		block.Format = FormatTable
	}

	return block
}

func parseCondition(raw string) (Condition, error) {
	if m := condContainRegex.FindStringSubmatch(raw); m != nil {
		return Condition{Field: strings.ToLower(m[1]), Op: OpContain, Value: m[2]}, nil
	}
	if m := condEqualRegex.FindStringSubmatch(raw); m != nil {
		return Condition{Field: strings.ToLower(m[1]), Op: OpEqual, Value: m[2]}, nil
	}
	if m := condContainsRegex.FindStringSubmatch(raw); m != nil {
		return Condition{Field: strings.ToLower(m[1]), Op: OpContains, Value: m[2]}, nil
	}
	return Condition{}, fmt.Errorf("cannot parse condition: %s", raw)
}
