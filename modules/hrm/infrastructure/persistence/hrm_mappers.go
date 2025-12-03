package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/currency"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/country"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	coremappers "github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/hrm/domain/aggregates/employee"
	"github.com/iota-uz/iota-sdk/modules/hrm/domain/entities/position"
	employeesqlc "github.com/iota-uz/iota-sdk/modules/hrm/infrastructure/sqlc/employee"
	positionsqlc "github.com/iota-uz/iota-sdk/modules/hrm/infrastructure/sqlc/position"
	"github.com/iota-uz/iota-sdk/pkg/money"

	"github.com/jackc/pgx/v5/pgtype"
)

type employeeRowData struct {
	ID                int32
	TenantID          pgtype.UUID
	FirstName         string
	LastName          string
	MiddleName        *string
	Email             string
	Phone             *string
	Salary            pgtype.Numeric
	SalaryCurrencyID  *string
	HourlyRate        pgtype.Numeric
	Coefficient       float64
	AvatarID          *int32
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
	PrimaryLanguage   *string
	SecondaryLanguage *string
	Tin               *string
	Pin               *string
	Notes             *string
	BirthDate         pgtype.Date
	HireDate          pgtype.Date
	ResignationDate   pgtype.Date
}

func employeeRowFromGet(row employeesqlc.GetEmployeeByIDRow) employeeRowData {
	return employeeRowData{
		ID:                row.ID,
		TenantID:          row.TenantID,
		FirstName:         row.FirstName,
		LastName:          row.LastName,
		MiddleName:        row.MiddleName,
		Email:             row.Email,
		Phone:             row.Phone,
		Salary:            row.Salary,
		SalaryCurrencyID:  row.SalaryCurrencyID,
		HourlyRate:        row.HourlyRate,
		Coefficient:       row.Coefficient,
		AvatarID:          row.AvatarID,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		PrimaryLanguage:   row.PrimaryLanguage,
		SecondaryLanguage: row.SecondaryLanguage,
		Tin:               row.Tin,
		Pin:               row.Pin,
		Notes:             row.Notes,
		BirthDate:         row.BirthDate,
		HireDate:          row.HireDate,
		ResignationDate:   row.ResignationDate,
	}
}

func employeeRowFromPaginated(row employeesqlc.ListEmployeesPaginatedRow) employeeRowData {
	return employeeRowData{
		ID:                row.ID,
		TenantID:          row.TenantID,
		FirstName:         row.FirstName,
		LastName:          row.LastName,
		MiddleName:        row.MiddleName,
		Email:             row.Email,
		Phone:             row.Phone,
		Salary:            row.Salary,
		SalaryCurrencyID:  row.SalaryCurrencyID,
		HourlyRate:        row.HourlyRate,
		Coefficient:       row.Coefficient,
		AvatarID:          row.AvatarID,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		PrimaryLanguage:   row.PrimaryLanguage,
		SecondaryLanguage: row.SecondaryLanguage,
		Tin:               row.Tin,
		Pin:               row.Pin,
		Notes:             row.Notes,
		BirthDate:         row.BirthDate,
		HireDate:          row.HireDate,
		ResignationDate:   row.ResignationDate,
	}
}

func employeeRowFromTenant(row employeesqlc.ListEmployeesByTenantRow) employeeRowData {
	return employeeRowData{
		ID:                row.ID,
		TenantID:          row.TenantID,
		FirstName:         row.FirstName,
		LastName:          row.LastName,
		MiddleName:        row.MiddleName,
		Email:             row.Email,
		Phone:             row.Phone,
		Salary:            row.Salary,
		SalaryCurrencyID:  row.SalaryCurrencyID,
		HourlyRate:        row.HourlyRate,
		Coefficient:       row.Coefficient,
		AvatarID:          row.AvatarID,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		PrimaryLanguage:   row.PrimaryLanguage,
		SecondaryLanguage: row.SecondaryLanguage,
		Tin:               row.Tin,
		Pin:               row.Pin,
		Notes:             row.Notes,
		BirthDate:         row.BirthDate,
		HireDate:          row.HireDate,
		ResignationDate:   row.ResignationDate,
	}
}

func toDomainEmployeeFromGetRow(row employeesqlc.GetEmployeeByIDRow) (employee.Employee, error) {
	return toDomainEmployeeFromRow(employeeRowFromGet(row))
}

func toDomainEmployeesFromPaginated(rows []employeesqlc.ListEmployeesPaginatedRow) ([]employee.Employee, error) {
	return mapEmployeeRows(rows, employeeRowFromPaginated)
}

func toDomainEmployeesFromTenantList(rows []employeesqlc.ListEmployeesByTenantRow) ([]employee.Employee, error) {
	return mapEmployeeRows(rows, employeeRowFromTenant)
}

func mapEmployeeRows[T any](rows []T, fn func(T) employeeRowData) ([]employee.Employee, error) {
	result := make([]employee.Employee, 0, len(rows))
	for _, row := range rows {
		entity, err := toDomainEmployeeFromRow(fn(row))
		if err != nil {
			return nil, err
		}
		result = append(result, entity)
	}
	return result, nil
}

func toDomainEmployeeFromRow(row employeeRowData) (employee.Employee, error) {
	tenantID, err := uuidFromPgUUID(row.TenantID)
	if err != nil {
		return nil, err
	}

	salaryFloat, err := numericToFloat64(row.Salary)
	if err != nil {
		return nil, err
	}

	currencyCode := stringValue(row.SalaryCurrencyID)
	if currencyCode == "" {
		return nil, errors.New("employee salary currency is empty")
	}
	curCode, err := currency.NewCode(currencyCode)
	if err != nil {
		return nil, err
	}
	salary := money.NewFromFloat(salaryFloat, string(curCode))

	email, err := internet.NewEmail(row.Email)
	if err != nil {
		return nil, err
	}

	tin, err := coremappers.ToDomainTin(nullStringFromPointer(row.Tin), country.Uzbekistan)
	if err != nil {
		return nil, err
	}
	pin, err := coremappers.ToDomainPin(nullStringFromPointer(row.Pin), country.Uzbekistan)
	if err != nil {
		return nil, err
	}

	var hireDate time.Time
	if row.HireDate.Valid {
		hireDate, err = timeFromDate(row.HireDate)
		if err != nil {
			return nil, err
		}
	}

	createdAt, err := timeFromTimestamptz(row.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := timeFromTimestamptz(row.UpdatedAt)
	if err != nil {
		return nil, err
	}
	avatarID := uintFromInt32Ptr(row.AvatarID)
	language := employee.NewLanguage(stringValue(row.PrimaryLanguage), stringValue(row.SecondaryLanguage))

	resignationDate, err := timePtrFromDate(row.ResignationDate)
	if err != nil {
		return nil, err
	}
	opts := []employee.Option{
		employee.WithAvatarID(avatarID),
		employee.WithResignationDate(resignationDate),
		employee.WithNotes(stringValue(row.Notes)),
		employee.WithCreatedAt(createdAt),
		employee.WithUpdatedAt(updatedAt),
	}

	if row.BirthDate.Valid {
		birthDate, err := timeFromDate(row.BirthDate)
		if err != nil {
			return nil, err
		}
		opts = append(opts, employee.WithBirthDate(birthDate))
	}

	entity := employee.NewWithID(
		uint(row.ID),
		tenantID,
		row.FirstName,
		row.LastName,
		stringValue(row.MiddleName),
		stringValue(row.Phone),
		email,
		salary,
		tin,
		pin,
		language,
		hireDate,
		opts...,
	)

	return entity, nil
}

func buildEmployeeInsertParams(entity employee.Employee, tenant uuid.UUID) (employeesqlc.CreateEmployeeParams, employeesqlc.CreateEmployeeMetaParams, error) {
	salary := entity.Salary()
	salaryFloat := float64(salary.Amount()) / 100
	salaryNumeric, err := numericFromFloat64(salaryFloat)
	if err != nil {
		return employeesqlc.CreateEmployeeParams{}, employeesqlc.CreateEmployeeMetaParams{}, err
	}
	hourlyRate, err := numericFromFloat64(0)
	if err != nil {
		return employeesqlc.CreateEmployeeParams{}, employeesqlc.CreateEmployeeMetaParams{}, err
	}

	lang := entity.Language()
	tenantPG := pgUUIDFromUUID(tenant)

	employeeParams := employeesqlc.CreateEmployeeParams{
		TenantID:         tenantPG,
		FirstName:        entity.FirstName(),
		LastName:         entity.LastName(),
		MiddleName:       stringPointer(entity.MiddleName()),
		Email:            entity.Email().Value(),
		Phone:            stringPointer(entity.Phone()),
		Salary:           salaryNumeric,
		SalaryCurrencyID: stringPointer(salary.Currency().Code),
		HourlyRate:       hourlyRate,
		Coefficient:      0,
		AvatarID:         int32PointerFromUint(entity.AvatarID()),
		CreatedAt:        timestamptzFromTime(entity.CreatedAt()),
		UpdatedAt:        timestamptzFromTime(entity.UpdatedAt()),
	}

	metaParams := employeesqlc.CreateEmployeeMetaParams{
		PrimaryLanguage:   stringPointer(lang.Primary()),
		SecondaryLanguage: stringPointer(lang.Secondary()),
		Tin:               stringPointer(entity.Tin().Value()),
		Pin:               stringPointer(entity.Pin().Value()),
		Notes:             stringPointer(entity.Notes()),
		BirthDate:         dateFromTime(entity.BirthDate()),
		HireDate:          dateFromTime(entity.HireDate()),
		ResignationDate:   dateFromPointer(entity.ResignationDate()),
	}

	return employeeParams, metaParams, nil
}

func buildEmployeeUpdateParams(entity employee.Employee, tenant uuid.UUID) (employeesqlc.UpdateEmployeeParams, employeesqlc.UpdateEmployeeMetaParams, error) {
	createParams, metaParams, err := buildEmployeeInsertParams(entity, tenant)
	if err != nil {
		return employeesqlc.UpdateEmployeeParams{}, employeesqlc.UpdateEmployeeMetaParams{}, err
	}

	updateParams := employeesqlc.UpdateEmployeeParams{
		FirstName:        createParams.FirstName,
		LastName:         createParams.LastName,
		MiddleName:       createParams.MiddleName,
		Email:            createParams.Email,
		Phone:            createParams.Phone,
		Salary:           createParams.Salary,
		SalaryCurrencyID: createParams.SalaryCurrencyID,
		HourlyRate:       createParams.HourlyRate,
		Coefficient:      createParams.Coefficient,
		AvatarID:         createParams.AvatarID,
		UpdatedAt:        createParams.UpdatedAt,
		TenantID:         createParams.TenantID,
	}

	updateMeta := employeesqlc.UpdateEmployeeMetaParams{
		PrimaryLanguage:   metaParams.PrimaryLanguage,
		SecondaryLanguage: metaParams.SecondaryLanguage,
		Tin:               metaParams.Tin,
		Pin:               metaParams.Pin,
		Notes:             metaParams.Notes,
		BirthDate:         metaParams.BirthDate,
		HireDate:          metaParams.HireDate,
		ResignationDate:   metaParams.ResignationDate,
	}

	return updateParams, updateMeta, nil
}

func toDomainPositionFromSQLC(row positionsqlc.Position) (*position.Position, error) {
	tenantID, err := uuidFromPgUUID(row.TenantID)
	if err != nil {
		return nil, err
	}
	createdAt, err := timeFromTimestamptz(row.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := timeFromTimestamptz(row.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &position.Position{
		ID:          uint(row.ID),
		TenantID:    tenantID.String(),
		Name:        row.Name,
		Description: stringValue(row.Description),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func toDomainPositionsFromSQLC(rows []positionsqlc.Position) ([]*position.Position, error) {
	result := make([]*position.Position, 0, len(rows))
	for _, row := range rows {
		entity, err := toDomainPositionFromSQLC(row)
		if err != nil {
			return nil, err
		}
		result = append(result, entity)
	}
	return result, nil
}

func buildPositionCreateParams(entity *position.Position, tenant uuid.UUID) positionsqlc.CreatePositionParams {
	return positionsqlc.CreatePositionParams{
		TenantID:    pgUUIDFromUUID(tenant),
		Name:        entity.Name,
		Description: stringPointer(entity.Description),
	}
}

func buildPositionUpdateParams(entity *position.Position, tenant uuid.UUID) positionsqlc.UpdatePositionParams {
	return positionsqlc.UpdatePositionParams{
		Name:        entity.Name,
		Description: stringPointer(entity.Description),
		ID:          int32(entity.ID),
		TenantID:    pgUUIDFromUUID(tenant),
	}
}

func uuidFromPgUUID(pg pgtype.UUID) (uuid.UUID, error) {
	if !pg.Valid {
		return uuid.Nil, errors.New("invalid uuid value")
	}
	return uuid.FromBytes(pg.Bytes[:])
}

func pgUUIDFromUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func numericFromFloat64(value float64) (pgtype.Numeric, error) {
	var num pgtype.Numeric
	if err := num.Scan(strconv.FormatFloat(value, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}, err
	}
	num.Valid = true
	return num, nil
}

func numericToFloat64(n pgtype.Numeric) (float64, error) {
	if !n.Valid {
		return 0, errors.New("numeric value is invalid")
	}
	v, err := n.Float64Value()
	if err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, errors.New("numeric value is not valid float")
	}
	return v.Float64, nil
}

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timeFromTimestamptz(ts pgtype.Timestamptz) (time.Time, error) {
	if !ts.Valid {
		return time.Time{}, errors.New("timestamp is invalid")
	}
	if ts.InfinityModifier != pgtype.Finite {
		return time.Time{}, fmt.Errorf("unsupported timestamptz infinity modifier: %v", ts.InfinityModifier)
	}
	return ts.Time, nil
}

func dateFromTime(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func dateFromPointer(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func timeFromDate(d pgtype.Date) (time.Time, error) {
	if !d.Valid {
		return time.Time{}, nil
	}
	if d.InfinityModifier != pgtype.Finite {
		return time.Time{}, fmt.Errorf("unsupported date infinity modifier: %v", d.InfinityModifier)
	}
	return d.Time, nil
}

func timePtrFromDate(d pgtype.Date) (*time.Time, error) {
	if !d.Valid {
		var unset *time.Time
		return unset, nil
	}
	t, err := timeFromDate(d)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	v := value
	return &v
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int32PointerFromUint(value uint) *int32 {
	if value == 0 {
		return nil
	}
	v := int32(value)
	return &v
}

func uintFromInt32Ptr(value *int32) uint {
	if value == nil || *value < 0 {
		return 0
	}
	return uint(*value)
}

func nullStringFromPointer(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
