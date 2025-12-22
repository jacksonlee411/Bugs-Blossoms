package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gerrors "github.com/go-faster/errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
	personsqlc "github.com/iota-uz/iota-sdk/modules/person/infrastructure/sqlc/person"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/constants"
)

type PersonRepository struct{}

func NewPersonRepository() person.Repository {
	return &PersonRepository{}
}

func (r *PersonRepository) GetPaginated(ctx context.Context, params *person.FindParams) ([]person.Person, int64, error) {
	if params == nil {
		params = &person.FindParams{}
	}

	tenantUUID, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, 0, err
	}

	queries, err := r.personQueries(ctx)
	if err != nil {
		return nil, 0, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	q := strings.TrimSpace(params.Q)
	rows, err := queries.ListPersonsPaginated(ctx, personsqlc.ListPersonsPaginatedParams{
		TenantID: pgTenantID,
		Column2:  q,
		Offset:   int32(offset),
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := queries.CountPersons(ctx, personsqlc.CountPersonsParams{
		TenantID: pgTenantID,
		Column2:  q,
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]person.Person, 0, len(rows))
	for _, row := range rows {
		if !row.TenantID.Valid || row.TenantID.Bytes != tenantUUID {
			continue
		}
		out = append(out, toDomainPersonFromRow(row))
	}

	return out, total, nil
}

func (r *PersonRepository) GetByUUID(ctx context.Context, personUUID uuid.UUID) (person.Person, error) {
	tenantUUID, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return person.Person{}, err
	}

	queries, err := r.personQueries(ctx)
	if err != nil {
		return person.Person{}, err
	}

	row, err := queries.GetPersonByUUID(ctx, personsqlc.GetPersonByUUIDParams{
		TenantID:   pgTenantID,
		PersonUuid: pgUUIDFromUUID(personUUID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return person.Person{}, person.ErrNotFound
		}
		return person.Person{}, err
	}

	if !row.TenantID.Valid || row.TenantID.Bytes != tenantUUID {
		return person.Person{}, person.ErrNotFound
	}

	return toDomainPersonFromRow(row), nil
}

func (r *PersonRepository) GetByPernr(ctx context.Context, pernr string) (person.Person, error) {
	tenantUUID, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return person.Person{}, err
	}

	queries, err := r.personQueries(ctx)
	if err != nil {
		return person.Person{}, err
	}

	row, err := queries.GetPersonByPernr(ctx, personsqlc.GetPersonByPernrParams{
		TenantID: pgTenantID,
		Pernr:    strings.TrimSpace(pernr),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return person.Person{}, person.ErrNotFound
		}
		return person.Person{}, err
	}

	if !row.TenantID.Valid || row.TenantID.Bytes != tenantUUID {
		return person.Person{}, person.ErrNotFound
	}

	return toDomainPersonFromRow(row), nil
}

func (r *PersonRepository) Create(ctx context.Context, p person.Person) (person.Person, error) {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return person.Person{}, err
	}

	queries, err := r.personQueries(ctx)
	if err != nil {
		return person.Person{}, err
	}

	row, err := queries.CreatePerson(ctx, personsqlc.CreatePersonParams{
		TenantID:    pgTenantID,
		Pernr:       strings.TrimSpace(p.Pernr()),
		DisplayName: strings.TrimSpace(p.DisplayName()),
		Status:      string(p.Status()),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return person.Person{}, person.ErrPernrTaken
		}
		return person.Person{}, fmt.Errorf("create person: %w", err)
	}

	return toDomainPersonFromRow(row), nil
}

func (r *PersonRepository) personQueries(ctx context.Context) (*personsqlc.Queries, error) {
	if configuration.Use().RLSEnforce == "enforce" {
		if ctx.Value(constants.TxKey) == nil {
			return nil, gerrors.New("rls enforced: person queries require an explicit transaction")
		}
	}
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	return personsqlc.New(tx), nil
}

func tenantIDs(ctx context.Context) (uuid.UUID, pgtype.UUID, error) {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return uuid.Nil, pgtype.UUID{}, fmt.Errorf("failed to get tenant from context: %w", err)
	}
	return tenantID, pgUUIDFromUUID(tenantID), nil
}

func toDomainPersonFromRow(row personsqlc.Person) person.Person {
	tenantID := uuid.Nil
	if row.TenantID.Valid {
		tenantID = row.TenantID.Bytes
	}
	personUUID := uuid.Nil
	if row.PersonUuid.Valid {
		personUUID = row.PersonUuid.Bytes
	}
	return person.Hydrate(
		tenantID,
		personUUID,
		row.Pernr,
		row.DisplayName,
		person.Status(row.Status),
		row.CreatedAt.Time,
		row.UpdatedAt.Time,
	)
}
