package helper

import "net/http"

const (
	MsgInternalServerError = "Internal Server Error"
	MsgBadRequest          = "Bad Request"
	MsgNotFound            = "Not Found"
	MsgUnauthorized        = "Unauthorized"
)

type AppError struct {
	Code    int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func NewBadRequestError(message string, err error) *AppError {
	if message == "" {
		message = MsgBadRequest
	}
	return NewAppError(http.StatusBadRequest, message, err)
}

func NewInternalServerError(message string, err error) *AppError {
	if message == "" {
		message = MsgInternalServerError
	}
	return NewAppError(http.StatusInternalServerError, message, err)
}

func NewNotFoundError(message string, err error) *AppError {
	if message == "" {
		message = MsgNotFound
	}
	return NewAppError(http.StatusNotFound, message, err)
}

func NewUnauthorizedError(message string, err error) *AppError {
	if message == "" {
		message = MsgUnauthorized
	}
	return NewAppError(http.StatusUnauthorized, message, err)
}
