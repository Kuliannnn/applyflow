// Package provider contains deterministic, visibly mock drafts; it makes no AI/network calls.
package provider

import (
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/resume"
)

type Mock struct{}

func (Mock) Draft(kind, role, company string, facts []resume.Fact) document.Content {
	if kind == "resume" {
		title := "Mock resume draft"
		c := document.Content{Kind: kind, Title: &title, Sections: make([]document.Section, 0)}
		for i, f := range facts {
			if i%50 == 0 {
				c.Sections = append(c.Sections, document.Section{Heading: "Confirmed experience and facts", Items: make([]document.EvidenceText, 0)})
			}
			j := len(c.Sections) - 1
			c.Sections[j].Items = append(c.Sections[j].Items, document.EvidenceText{Text: f.Text, FactIDs: []string{f.ID}})
		}
		return c
	}
	salutation, closing := "Dear Hiring Team,", "Sincerely,"
	intro := "Mock draft for this opportunity. Review and edit before use."
	if role != "" && company != "" {
		intro = "Mock draft for the " + role + " role at " + company + ". Review and edit before use."
	}
	paragraphs := []document.EvidenceText{{Text: intro, FactIDs: []string{}}}
	for i, f := range facts {
		if i == 10 {
			break
		}
		paragraphs = append(paragraphs, document.EvidenceText{Text: f.Text, FactIDs: []string{f.ID}})
	}
	return document.Content{Kind: kind, Salutation: &salutation, Closing: &closing, Paragraphs: paragraphs}
}
