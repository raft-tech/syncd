package helpers

import "errors"

type Error interface {
	error
	Code() int
}

func NewError(msg string, exitCode int) Error {
	return &wrappedError{
		error:    errors.New(msg),
		exitCode: exitCode,
	}
}

func WrapError(err error, exitCode int) Error {
	return &wrappedError{
		error:    err,
		exitCode: exitCode,
	}
}

type wrappedError struct {
	error
	exitCode int
}

func (e *wrappedError) Code() int {
	if e == nil {
		return 0
	} else {
		return e.exitCode
	}
}
