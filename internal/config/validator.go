package config

import (
	"AtoiTalkAPI/internal/constant"
	"unicode"

	"github.com/go-playground/validator/v10"
)

func NewValidator() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("otp_mode", validateOTPMode)
	_ = v.RegisterValidation("password_complexity", validatePasswordComplexity)
	return v
}

func validateOTPMode(fl validator.FieldLevel) bool {
	mode := fl.Field().String()
	return mode == constant.OTPModeRegister || mode == constant.OTPModeReset
}

func validatePasswordComplexity(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	var (
		hasUpper  bool
		hasLower  bool
		hasNumber bool
		hasSymbol bool
	)
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSymbol = true
		}
	}
	return hasUpper && hasLower && hasNumber && hasSymbol
}
