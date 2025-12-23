package person

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/iota-uz/iota-sdk/pkg/constants"
	"github.com/iota-uz/iota-sdk/pkg/intl"
	"github.com/iota-uz/iota-sdk/pkg/serrors"
)

type CreateDTO struct {
	Pernr       string `json:"pernr" validate:"required"`
	DisplayName string `json:"display_name" validate:"required"`
}

func (d *CreateDTO) Normalize() {
	d.Pernr = strings.TrimSpace(d.Pernr)
	d.DisplayName = strings.TrimSpace(d.DisplayName)
}

func (d *CreateDTO) Ok(ctx context.Context) (map[string]string, bool) {
	l, ok := intl.UseLocalizer(ctx)
	if !ok {
		panic(intl.ErrNoLocalizer)
	}

	d.Normalize()

	errs := constants.Validate.Struct(d)
	if errs == nil {
		return map[string]string{}, true
	}

	validationErrors := make(serrors.ValidationErrors)
	validatorErrs := errs.(validator.ValidationErrors)
	getFieldLocaleKey := func(field string) string {
		switch field {
		case "Pernr", "DisplayName":
			return fmt.Sprintf("Person.Fields.%s", field)
		default:
			return ""
		}
	}
	for field, err := range serrors.ProcessValidatorErrors(validatorErrs, getFieldLocaleKey) {
		validationErrors[field] = err
	}

	return serrors.LocalizeValidationErrors(validationErrors, l), false
}
