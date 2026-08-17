package utils

import (
	"strings"

	"github.com/go-playground/validator/v10"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// ValidateStruct runs go-playground/validator over a DTO and converts failures
// into a field-wise *AppError. Called by handlers BEFORE the service layer.
func ValidateStruct(s any) *apperrors.AppError {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}
	verrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return apperrors.BadRequest("Invalid request payload")
	}
	fields := make(map[string]string, len(verrs))
	for _, fe := range verrs {
		fields[strings.ToLower(fe.Field())] = messageFor(fe)
	}
	return apperrors.Validation(fields)
}

func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Must be a valid email address"
	case "min":
		return "Must be at least " + fe.Param() + " characters"
	case "max":
		return "Must be at most " + fe.Param() + " characters"
	case "oneof":
		return "Must be one of: " + fe.Param()
	case "uuid":
		return "Must be a valid UUID"
	default:
		return "Invalid value"
	}
}
