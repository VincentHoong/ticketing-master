package utils

import (
	"encoding/json"
	"errors"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

var ErrMalformedRequestBody = errors.New("request body malformed")

func DecodeRequestBody[T any](w http.ResponseWriter, r *http.Request, body *T) error {
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return ErrMalformedRequestBody
	}
	return nil
}

func WriteErrorResponse(w http.ResponseWriter, status int, err error) {
	encoded, marshalErr := json.Marshal(&errorResponse{
		Error: err.Error(),
	})
	if marshalErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(encoded)
}

func WriteJSONResponse(w http.ResponseWriter, status int, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data != nil {
		w.Write(encoded)
	}
}
