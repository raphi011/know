package pipeline

import (
	"context"
	"fmt"

	"github.com/raphi011/know/internal/models"
)

// PDFChunker splits PDFs into page-based chunks.
// Currently a stub — full implementation requires a PDF library (e.g. pdfcpu).
type PDFChunker struct{}

func (p *PDFChunker) Chunk(_ context.Context, file *models.File, _ ChunkConfig) ([]ChunkResult, error) {
	if len(file.Data) == 0 {
		return nil, nil
	}

	// TODO: implement page-based splitting with a PDF library.
	// For now, create a single chunk with the full PDF data.
	return []ChunkResult{{
		Data:      file.Data,
		MimeType:  file.MimeType,
		Position:  0,
		SourceLoc: fmt.Sprintf("page 1-%d", 1), // placeholder
	}}, nil
}
