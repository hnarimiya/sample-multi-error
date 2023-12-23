package main

import (
	"encoding/json"
	"errors"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	mw "github.com/oapi-codegen/nethttp-middleware"
	"net/http"
	"sample/server"
	"strconv"
	"strings"
)

func main() {
	swagger, err := server.GetSwagger()
	if err != nil {
		panic(err)
	}
	swagger.Servers = nil

	options := mw.Options{
		Options: openapi3filter.Options{
			MultiError: true,
		},
		ErrorHandler:      errorHandler(),
		MultiErrorHandler: multiErrorHandler(),
	}

	router := chi.NewRouter()
	router.Use(mw.OapiRequestValidatorWithOptions(swagger, &options))

	server.HandlerWithOptions(&Controller{}, server.ChiServerOptions{
		BaseRouter: router,
	})

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}
	if err := srv.ListenAndServe(); err != nil {
		return
	}
}

type Controller struct {
}

func (c *Controller) PostContent(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	b, _ := json.Marshal(body)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(b)
}

func errorHandler() func(w http.ResponseWriter, message string, statusCode int) {
	return func(w http.ResponseWriter, message string, statusCode int) {
		var body server.ErrorResponse
		if strings.Index(message, "multi_error:") == 0 {
			_ = json.Unmarshal([]byte(strings.SplitN(message, ":", 2)[1]), &body)
		} else {
			body.Message = message
		}
		writeJSON(w, statusCode, body)
	}
}

type FieldError struct {
	Field string `json:"field"`
	Error string `json:"error"`
}

type ValidationError struct {
	Message string       `json:"message"`
	Errors  []FieldError `json:"errors"`
}

func (e ValidationError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return e.Message
	}
	return "multi_error:" + string(b)
}

func multiErrorHandler() func(me openapi3.MultiError) (int, error) {
	return func(me openapi3.MultiError) (int, error) {
		var securityRequirementsError *openapi3filter.SecurityRequirementsError
		if me.As(&securityRequirementsError) {
			return http.StatusUnauthorized, me
		}

		var requestErr *openapi3filter.RequestError
		if !me.As(&requestErr) {
			return http.StatusBadRequest, me
		}

		var errorList openapi3.MultiError
		ok := errors.As(requestErr.Err, &errorList)
		if !ok {
			return http.StatusBadRequest, me
		}

		var fieldErrors []FieldError
		for _, err := range errorList {
			var e *openapi3.SchemaError
			if errors.As(err, &e) {
				fieldErrors = append(fieldErrors, FieldError{
					Field: convertJSONPath(e.JSONPointer()),
					Error: e.Reason,
				})
			}
		}

		validationError := ValidationError{
			Message: "validation error",
			Errors:  fieldErrors,
		}
		return http.StatusBadRequest, validationError
	}
}

func convertJSONPath(ptrs []string) string {
	var paths []string
	for i, ptr := range ptrs {
		if _, err := strconv.Atoi(ptr); err == nil {
			paths = append(paths, "["+ptr+"]")
		} else {
			if i == 0 {
				paths = append(paths, ptr)
			} else {
				paths = append(paths, "."+ptr)
			}
		}
	}
	return strings.Join(paths, "")
}
