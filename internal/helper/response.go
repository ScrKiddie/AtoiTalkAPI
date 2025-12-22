package helper

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Errors  interface{} `json:"errors,omitempty"`
}

func WriteJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func WriteSuccess(w http.ResponseWriter, message string, data interface{}) {
	WriteJSON(w, http.StatusOK, Response{
		Status:  true,
		Message: message,
		Data:    data,
	})
}

func WriteError(w http.ResponseWriter, err error) {
	appErr, ok := err.(*AppError)
	if !ok {
		appErr = NewInternalServerError("Internal Server Error", err)
	}

	WriteJSON(w, appErr.Code, Response{
		Status:  false,
		Message: appErr.Message,
		Errors:  appErr.Err,
	})
}
