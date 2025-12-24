package helper

import (
	"encoding/json"
	"net/http"
)

type ResponseSuccess struct {
	Data interface{} `json:"data"`
}

type ResponseError struct {
	Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func WriteSuccess(w http.ResponseWriter, data interface{}) {
	if data == nil {
		data = ""
	}
	WriteJSON(w, http.StatusOK, ResponseSuccess{
		Data: data,
	})
}

func WriteError(w http.ResponseWriter, err error) {
	appErr, ok := err.(*AppError)
	if !ok {
		appErr = NewInternalServerError("Internal Server Error")
	}

	WriteJSON(w, appErr.Code, ResponseError{
		Error: appErr.Message,
	})
}
