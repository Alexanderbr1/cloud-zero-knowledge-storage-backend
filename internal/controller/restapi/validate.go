package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return f.Name
		}
		return name
	})
}

func ValidateStruct(v any) error {
	return validate.Struct(v)
}

func WriteValidationError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fields := make([]map[string]string, 0, len(verrs))
		for _, fe := range verrs {
			fields = append(fields, map[string]string{
				"field": fe.Field(),
				"tag":   fe.Tag(),
				"value": fmt.Sprint(fe.Value()),
			})
		}
		WriteJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "validation failed",
			"fields": fields,
		})
		return
	}

	var inv *validator.InvalidValidationError
	if errors.As(err, &inv) {
		WriteError(w, http.StatusBadRequest, "bad request")
		return
	}

	WriteError(w, http.StatusBadRequest, "bad request")
}
