package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return f.Name
		}
		return name
	})
	return v
}()

func ValidateStruct(v any) error {
	return validate.Struct(v)
}

func WriteValidationError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fields := make([]fieldError, 0, len(verrs))
		for _, fe := range verrs {
			fields = append(fields, fieldError{
				Field: fe.Field(),
				Tag:   fe.Tag(),
				Value: fmt.Sprint(fe.Value()),
			})
		}
		WriteJSON(w, http.StatusBadRequest, validationErrorResponse{
			Error:  "validation failed",
			Fields: fields,
		})
		return
	}

	WriteError(w, http.StatusBadRequest, "bad request")
}
