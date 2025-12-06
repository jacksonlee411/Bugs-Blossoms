package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/constants"
)

func TestAuthenticationLogRepository_List_UsesTenantAndMapsRows(t *testing.T) {
	tenantID := uuid.New()
	queryCalled := false
	now := time.Now()

	tx := &stubTx{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			queryCalled = true
			require.Contains(t, sql, "FROM authentication_logs")
			require.Equal(t, tenantID, args[0])
			return &stubRows{data: [][]any{
				{uint(1), tenantID.String(), uint(9), "127.0.0.1", "ua", now},
			}}, nil
		},
	}

	ctx := context.WithValue(composables.WithTenantID(context.Background(), tenantID), constants.TxKey, tx)
	repo := NewAuthenticationLogRepository()

	result, err := repo.List(ctx, &authenticationlog.FindParams{Limit: 10, Offset: 5})
	require.NoError(t, err)
	require.True(t, queryCalled)
	require.Len(t, result, 1)
	require.Equal(t, tenantID, result[0].TenantID)
	require.Equal(t, uint(9), result[0].UserID)
	require.Equal(t, now, result[0].CreatedAt)
}

func TestAuthenticationLogRepository_Count_UsesTenantFilter(t *testing.T) {
	tenantID := uuid.New()

	tx := &stubTx{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "authentication_logs")
			require.Equal(t, tenantID, args[0])
			return stubRow{
				scan: func(dest ...any) error {
					require.Len(t, dest, 1)
					*dest[0].(*int64) = 3
					return nil
				},
			}
		},
	}

	ctx := context.WithValue(composables.WithTenantID(context.Background(), tenantID), constants.TxKey, tx)
	repo := NewAuthenticationLogRepository()

	count, err := repo.Count(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

func TestAuthenticationLogRepository_Create_FillsTenantAndTimestamp(t *testing.T) {
	tenantID := uuid.New()
	tx := &stubTx{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "INSERT INTO authentication_logs")
			require.Equal(t, tenantID.String(), args[0])
			require.Equal(t, "127.0.0.1", args[2])
			require.IsType(t, time.Time{}, args[4])
			createdAt := args[4].(time.Time)

			return stubRow{
				scan: func(dest ...any) error {
					require.Len(t, dest, 2)
					*dest[0].(*uint) = 11
					*dest[1].(*time.Time) = createdAt
					return nil
				},
			}
		},
	}

	ctx := context.WithValue(composables.WithTenantID(context.Background(), tenantID), constants.TxKey, tx)
	repo := NewAuthenticationLogRepository()

	logEntry := &authenticationlog.AuthenticationLog{
		IP:        "127.0.0.1",
		UserAgent: "ua",
	}
	err := repo.Create(ctx, logEntry)
	require.NoError(t, err)
	require.Equal(t, tenantID, logEntry.TenantID)
	require.NotZero(t, logEntry.CreatedAt)
	require.Equal(t, uint(0), logEntry.UserID)
}

func TestActionLogRepository_List_UsesTenantAndMapsRows(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now()

	userID := uint(1)
	tx := &stubTx{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "FROM action_logs")
			require.Equal(t, tenantID, args[0])
			before := json.RawMessage(`{"from":"a"}`)
			after := json.RawMessage(`{"to":"b"}`)
			return &stubRows{data: [][]any{
				{uint(7), tenantID.String(), &userID, "GET", "/logs", before, after, "ua", "1.1.1.1", now},
			}}, nil
		},
	}

	ctx := context.WithValue(composables.WithTenantID(context.Background(), tenantID), constants.TxKey, tx)
	repo := NewActionLogRepository()

	result, err := repo.List(ctx, &actionlog.FindParams{Limit: 5})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, tenantID, result[0].TenantID)
	require.Equal(t, "GET", result[0].Method)
	require.Equal(t, "/logs", result[0].Path)
	require.Equal(t, now, result[0].CreatedAt)
}

func TestActionLogRepository_Count_UsesTenantFilter(t *testing.T) {
	tenantID := uuid.New()

	tx := &stubTx{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "action_logs")
			require.Equal(t, tenantID, args[0])
			return stubRow{
				scan: func(dest ...any) error {
					*dest[0].(*int64) = 8
					return nil
				},
			}
		},
	}

	ctx := context.WithValue(composables.WithTenantID(context.Background(), tenantID), constants.TxKey, tx)
	repo := NewActionLogRepository()

	count, err := repo.Count(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, int64(8), count)
}

func TestActionLogRepository_Create_FillsTenantAndTimestamp(t *testing.T) {
	tenantID := uuid.New()
	tx := &stubTx{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			require.Contains(t, sql, "INSERT INTO action_logs")
			require.Equal(t, tenantID.String(), args[0])
			require.Equal(t, "POST", args[1])
			require.Equal(t, "/logs", args[2])
			require.IsType(t, time.Time{}, args[8])
			createdAt := args[8].(time.Time)

			return stubRow{
				scan: func(dest ...any) error {
					require.Len(t, dest, 2)
					*dest[0].(*uint) = 55
					*dest[1].(*time.Time) = createdAt
					return nil
				},
			}
		},
	}

	ctx := context.WithValue(composables.WithTenantID(context.Background(), tenantID), constants.TxKey, tx)
	repo := NewActionLogRepository()

	logEntry := &actionlog.ActionLog{
		Method:    "POST",
		Path:      "/logs",
		UserAgent: "ua",
		IP:        "1.1.1.1",
	}
	err := repo.Create(ctx, logEntry)
	require.NoError(t, err)
	require.Equal(t, tenantID, logEntry.TenantID)
	require.NotZero(t, logEntry.CreatedAt)
}

type stubTx struct {
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *stubTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("copy not implemented")
}

func (s *stubTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	var results pgx.BatchResults
	return results
}

func (s *stubTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *stubTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if s.queryFunc == nil {
		return nil, errors.New("query not implemented")
	}
	return s.queryFunc(ctx, sql, args...)
}

func (s *stubTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFunc == nil {
		return stubRow{scan: func(dest ...any) error { return errors.New("query row not implemented") }}
	}
	return s.queryRowFunc(ctx, sql, args...)
}

type stubRows struct {
	data [][]any
	idx  int
	err  error
}

func (r *stubRows) Next() bool {
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *stubRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.data) {
		return errors.New("no current row to scan")
	}
	row := r.data[r.idx-1]
	if len(dest) != len(row) {
		return fmt.Errorf("destination length %d does not match row length %d", len(dest), len(row))
	}
	for i, target := range dest {
		switch v := target.(type) {
		case *uint:
			*v = row[i].(uint)
		case *uuid.UUID:
			*v = row[i].(uuid.UUID)
		case *string:
			*v = row[i].(string)
		case *time.Time:
			*v = row[i].(time.Time)
		case *json.RawMessage:
			raw := row[i].(json.RawMessage)
			*v = raw
		case *[]byte:
			switch val := row[i].(type) {
			case []byte:
				*v = val
			case json.RawMessage:
				*v = []byte(val)
			default:
				return fmt.Errorf("unsupported []byte source %T", row[i])
			}
		case **uint:
			ptr := row[i].(*uint)
			*v = ptr
		default:
			return fmt.Errorf("unsupported scan target %T", target)
		}
	}
	return nil
}

func (r *stubRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.data) {
		return nil, errors.New("no current row")
	}
	return r.data[r.idx-1], nil
}

func (r *stubRows) RawValues() [][]byte { return nil }
func (r *stubRows) Err() error          { return r.err }
func (r *stubRows) Close()              {}
func (r *stubRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Conn() *pgx.Conn                              { return nil }

type stubRow struct {
	scan func(dest ...any) error
}

func (r stubRow) Scan(dest ...any) error {
	if r.scan == nil {
		return errors.New("scan not implemented")
	}
	return r.scan(dest...)
}
