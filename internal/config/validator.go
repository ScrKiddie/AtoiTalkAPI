package config

import (
	"AtoiTalkAPI/internal/constant"
	"github.com/go-playground/validator/v10"
)

func NewValidator() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("temp_code_mode", validateTempCodeMode)
	return v
}

func validateTempCodeMode(fl validator.FieldLevel) bool {
	mode := fl.Field().String()
	return mode == constant.TempCodeModeRegister || mode == constant.TempCodeModeReset
}
