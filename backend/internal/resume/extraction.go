package resume

import "context"

type Extraction struct {
	Facts     []Fact `json:"facts"`
	PageCount int    `json:"page_count"`
}
type TextExtractor interface {
	Extract(context.Context, string, []byte) (Extraction, error)
}
type ExtractionError string

func (e ExtractionError) Error() string { return string(e) }
