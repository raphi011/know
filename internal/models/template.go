package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Template represents an output rendering template for synthesizing knowledge.
// Templates are used to generate structured documents from accumulated knowledge.
type Template struct {
	ID surrealmodels.RecordID `json:"id"`

	// Identity
	Name        string  `json:"name"`                  // "Peer Review", "Service Summary", "Weekly Report"
	Description *string `json:"description,omitempty"` // Short description of template purpose

	// Content
	Content string `json:"content"` // Markdown template with sections to fill

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateInput is the input structure for creating templates.
type TemplateInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Content     string  `json:"content"`
}

// TemplateUpdate is the input structure for updating templates.
type TemplateUpdate struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Content     *string `json:"content,omitempty"`
}

// DefaultTemplates returns the set of built-in templates.
func DefaultTemplates() []TemplateInput {
	return []TemplateInput{
		{
			Name:        "Peer Review",
			Description: ptr("Generate a comprehensive peer review from gathered knowledge"),
			Content: `# Peer Review: {name}

## Performance Summary
{overall assessment based on gathered feedback}

## Strengths
- {strength 1 with specific example}
- {strength 2 with specific example}

## Areas for Growth
- {area 1 with context}

## Key Contributions
| Project | Contribution | Impact |
|---------|--------------|--------|
| {project} | {contribution} | {impact} |

## Feedback from Others
{synthesized feedback from notes/observations}

## Recommendation
{promotion/growth recommendation based on evidence}
`,
		},
		{
			Name:        "Service Summary",
			Description: ptr("Generate a summary of a service from gathered knowledge"),
			Content: `# Service Summary: {name}

## Overview
{what this service does}

## Current Status
- Health: {based on recent observations}
- Recent incidents: {from notes}

## Key Metrics
{if captured in knowledge}

## Open Issues
{tasks/bugs related to this service}

## Team
{people who work on this service}

## Dependencies
{services this depends on / services that depend on this}
`,
		},
		{
			Name:        "Meeting Notes Summary",
			Description: ptr("Summarize meeting notes and action items"),
			Content: `# Meeting Summary: {topic}

## Date
{meeting date}

## Attendees
{list of participants}

## Key Decisions
- {decision 1}
- {decision 2}

## Action Items
| Owner | Action | Due Date |
|-------|--------|----------|
| {owner} | {action} | {due} |

## Discussion Points
{main topics discussed}

## Follow-up Required
{items needing follow-up}
`,
		},
	}
}

func ptr(s string) *string {
	return &s
}
