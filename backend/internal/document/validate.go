package document

import (
	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/studio"
	"strings"
)

func Validate(c Content, kind string, allowed map[string]bool) error {
	if c.Kind != kind {
		return studio.ErrInvalid
	}
	var items []EvidenceText
	switch kind {
	case "resume":
		if c.Title == nil || !intake.ValidText(*c.Title, 200) || len(c.Sections) < 1 || len(c.Sections) > 20 || c.Salutation != nil || c.Closing != nil || c.Paragraphs != nil {
			return studio.ErrInvalid
		}
		for _, s := range c.Sections {
			if !intake.ValidText(s.Heading, 120) || len(s.Items) < 1 || len(s.Items) > 50 {
				return studio.ErrInvalid
			}
			items = append(items, s.Items...)
		}
	case "cover_letter":
		if c.Salutation == nil || c.Closing == nil || !intake.ValidText(*c.Salutation, 200) || !intake.ValidText(*c.Closing, 200) || len(c.Paragraphs) < 1 || len(c.Paragraphs) > 12 || c.Title != nil || c.Sections != nil {
			return studio.ErrInvalid
		}
		items = c.Paragraphs
	default:
		return studio.ErrInvalid
	}
	for _, item := range items {
		if !intake.ValidText(item.Text, 4000) || item.FactIDs == nil || len(item.FactIDs) > 200 {
			return studio.ErrInvalid
		}
		seen := map[string]bool{}
		for _, id := range item.FactIDs {
			k := strings.ToLower(id)
			if !security.ValidUUID(id) || seen[k] || (allowed != nil && !allowed[k]) {
				return studio.ErrInvalid
			}
			seen[k] = true
		}
	}
	return nil
}
