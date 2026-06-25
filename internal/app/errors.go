package app

import (
	"errors"
	"fmt"
)

const (
	exitUsage      = 2
	exitAuth       = 3
	exitAPI        = 4
	exitFilesystem = 5
	exitInternal   = 1
)

type CLIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	Err     error  `json:"-"`
}

func (e *CLIError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func usageError(format string, args ...any) *CLIError {
	return &CLIError{Code: "usage_error", Message: fmt.Sprintf(format, args...), Status: exitUsage}
}

func authError(format string, args ...any) *CLIError {
	return &CLIError{Code: "auth_error", Message: fmt.Sprintf(format, args...), Status: exitAuth}
}

func apiError(format string, args ...any) *CLIError {
	return &CLIError{Code: "api_error", Message: fmt.Sprintf(format, args...), Status: exitAPI}
}

func fsError(format string, args ...any) *CLIError {
	return &CLIError{Code: "filesystem_error", Message: fmt.Sprintf(format, args...), Status: exitFilesystem}
}

func wrapError(code, message string, status int, err error) *CLIError {
	return &CLIError{Code: code, Message: message, Status: status, Err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		if cliErr.Status != 0 {
			return cliErr.Status
		}
	}
	return exitInternal
}
