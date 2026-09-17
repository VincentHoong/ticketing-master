package utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"ticketing-master/logging"
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

func WriteErrorResponse(w http.ResponseWriter, r *http.Request, status int, err error) {
	encoded, marshalErr := json.Marshal(&errorResponse{
		Error: err.Error(),
	})
	if marshalErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		bytesWritten, err := w.Write([]byte(`{"error":"internal server error"}`))
		if err != nil {
			logging.FromContext(r.Context()).Error("Failed to return failed error response", "bytesWritten", bytesWritten, "error", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	bytesWritten, err := w.Write(encoded)
	if err != nil {
		logging.FromContext(r.Context()).Error("Failed to return error response", "bytesWritten", bytesWritten, "error", err)
	}
}

func WriteJSONResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		WriteErrorResponse(w, r, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data != nil {
		bytesWritten, err := w.Write(encoded)
		if err != nil {
			logging.FromContext(r.Context()).Error("Failed to return json response", "bytesWritten", bytesWritten, "error", err)
		}
	}
}
