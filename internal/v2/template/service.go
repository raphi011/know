package template

import (
	"context"

	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"
)

// Service manages template CRUD operations.
type Service struct {
	db *v2db.Client
}

// NewService creates a new template service.
func NewService(db *v2db.Client) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, input models.TemplateInput) (*models.Template, error) {
	return s.db.CreateTemplate(ctx, input)
}

func (s *Service) Get(ctx context.Context, id string) (*models.Template, error) {
	return s.db.GetTemplate(ctx, id)
}

func (s *Service) List(ctx context.Context, vaultID *string) ([]models.Template, error) {
	return s.db.ListTemplates(ctx, vaultID)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.DeleteTemplate(ctx, id)
}
