package restapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// requestValidate — один экземпляр на процесс (кэш тегов), см. https://pkg.go.dev/github.com/go-playground/validator/v10
var (
	requestValidate     *validator.Validate
	initRequestValidate sync.Once
)

func validatorInstance() *validator.Validate {
	initRequestValidate.Do(func() {
		v := validator.New(validator.WithRequiredStructEnabled())
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
			if name == "-" || name == "" {
				return f.Name
			}
			return name
		})
		// std_base64 — стандартный base64; пустая строка пропускается (обязательность — через required).
		_ = v.RegisterValidation("std_base64", func(fl validator.FieldLevel) bool {
			s := fl.Field().String()
			if s == "" {
				return true
			}
			_, err := base64.StdEncoding.DecodeString(s)
			return err == nil
		})
		requestValidate = v
	})
	return requestValidate
}

// ValidateStruct проверяет DTO тегами validate; при ошибке пишет 400 и возвращает false.
func ValidateStruct(w http.ResponseWriter, v any) bool {
	err := validatorInstance().Struct(v)
	if err == nil {
		return true
	}

	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fields := make([]map[string]string, 0, len(verrs))
		for _, fe := range verrs {
			fields = append(fields, map[string]string{
				"field": fe.Field(),
				"tag":   fe.Tag(),
			})
		}
		WriteJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "validation failed",
			"fields": fields,
		})
		return false
	}

	var inv *validator.InvalidValidationError
	if errors.As(err, &inv) {
		WriteError(w, http.StatusBadRequest, "bad request")
		return false
	}

	WriteError(w, http.StatusBadRequest, "bad request")
	return false
}
