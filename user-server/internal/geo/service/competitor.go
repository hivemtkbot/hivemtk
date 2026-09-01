package service

import (
	"context"
	"encoding/json"
	"fmt"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/utils/logger"
)

type GeoCompetitorService struct {
	repo repository.GeoCompetitorRepository
}

func NewGeoCompetitorService(r repository.GeoCompetitorRepository) *GeoCompetitorService {
	return &GeoCompetitorService{repo: r}
}

type CompetitorDTO struct {
	ID       uint     `json:"id,omitempty"`
	Name     string   `json:"name"`
	Domain   string   `json:"domain"`
	Paths    []string `json:"paths"`
	Category string   `json:"category"`
	Priority int      `json:"priority"`
	Status   string   `json:"status"`
	Notes    string   `json:"notes"`
}

func (s *GeoCompetitorService) ListCompetitors(ctx context.Context, status string) ([]CompetitorDTO, error) {
	list, err := s.repo.List(ctx, status)
	if err != nil {
		return nil, err
	}
	out := make([]CompetitorDTO, 0, len(list))
	for _, c := range list {
		out = append(out, toCompetitorDTO(c))
	}
	return out, nil
}

func (s *GeoCompetitorService) GetCompetitor(ctx context.Context, id uint) (*CompetitorDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	dto := toCompetitorDTO(c)
	return &dto, nil
}

func (s *GeoCompetitorService) CreateCompetitor(ctx context.Context, dto CompetitorDTO) (*CompetitorDTO, error) {
	if dto.Name == "" || dto.Domain == "" {
		return nil, fmt.Errorf("name and domain are required")
	}
	c := &model.GeoCompetitor{
		Name:     dto.Name,
		Domain:   dto.Domain,
		Paths:    strSliceToJSON(dto.Paths),
		Category: dto.Category,
		Priority: dto.Priority,
		Status:   dto.Status,
		Notes:    dto.Notes,
	}
	if c.Priority == 0 {
		c.Priority = 5
	}
	if c.Status == "" {
		c.Status = "active"
	}
	if c.Category == "" {
		c.Category = "direct"
	}
	if err := s.repo.Create(ctx, c); err != nil {
		logger.Error(err, "CreateCompetitor failed")
		return nil, err
	}
	dtoOut := toCompetitorDTO(c)
	return &dtoOut, nil
}

func (s *GeoCompetitorService) UpdateCompetitor(ctx context.Context, id uint, dto CompetitorDTO) (*CompetitorDTO, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Name = dto.Name
	c.Domain = dto.Domain
	c.Paths = strSliceToJSON(dto.Paths)
	c.Category = dto.Category
	c.Priority = dto.Priority
	c.Status = dto.Status
	c.Notes = dto.Notes
	if err := s.repo.Update(ctx, c); err != nil {
		logger.Error(err, "UpdateCompetitor failed")
		return nil, err
	}
	dtoOut := toCompetitorDTO(c)
	return &dtoOut, nil
}

func (s *GeoCompetitorService) DeleteCompetitor(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

func toCompetitorDTO(c *model.GeoCompetitor) CompetitorDTO {
	return CompetitorDTO{
		ID:       c.ID,
		Name:     c.Name,
		Domain:   c.Domain,
		Paths:    jsonToStrSlice(c.Paths),
		Category: c.Category,
		Priority: c.Priority,
		Status:   c.Status,
		Notes:    c.Notes,
	}
}

// strSliceToJSON []string → datatypes.JSON
func strSliceToJSON(in []string) []byte {
	if in == nil {
		return []byte("[]")
	}
	b, _ := json.Marshal(in)
	return b
}

// jsonToStrSlice datatypes.JSON → []string
func jsonToStrSlice(in []byte) []string {
	if len(in) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(in, &out); err != nil {
		return []string{}
	}
	return out
}
