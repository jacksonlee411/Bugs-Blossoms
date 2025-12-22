package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
)

type PersonService struct {
	repo person.Repository
}

func NewPersonService(repo person.Repository) *PersonService {
	return &PersonService{repo: repo}
}

func (s *PersonService) GetPaginated(ctx context.Context, params *person.FindParams) ([]person.Person, int64, error) {
	if params != nil {
		params.Q = strings.TrimSpace(params.Q)
	}
	return s.repo.GetPaginated(ctx, params)
}

func (s *PersonService) GetByUUID(ctx context.Context, personUUID uuid.UUID) (person.Person, error) {
	return s.repo.GetByUUID(ctx, personUUID)
}

func (s *PersonService) GetByPernr(ctx context.Context, pernr string) (person.Person, error) {
	return s.repo.GetByPernr(ctx, pernr)
}

func (s *PersonService) Create(ctx context.Context, dto *person.CreateDTO) (person.Person, error) {
	if dto == nil {
		return person.Person{}, errors.New("missing dto")
	}
	dto.Normalize()
	entity := person.New(uuid.Nil, dto.Pernr, dto.DisplayName)
	created, err := s.repo.Create(ctx, entity)
	if err != nil {
		return person.Person{}, err
	}
	return created, nil
}
