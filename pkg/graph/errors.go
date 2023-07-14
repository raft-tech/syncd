package graph

import "errors"

type errType uint

const (
	unknownError errType = iota
	databaseError
	modelError
	dataError
)

var (
	noDataError = Error{
		error:   errors.New("no matching row"),
		errType: modelError,
	}
	unexpectedRowCountError = Error{
		error:   errors.New("unexpected row count"),
		errType: modelError,
	}
	malformedDataError = Error{
		error:   errors.New("malformed data"),
		errType: dataError,
	}
)

func NoDataError() Error {
	return noDataError
}

func UnexpectedRowCountError() Error {
	return unexpectedRowCountError
}

func MalformedDataError() Error {
	return malformedDataError
}

type Error struct {
	error
	errType
	wrapped []error
}

func NewDatabaseError(err error) *Error {
	return &Error{err, databaseError, nil}
}

func NewModelError(err error) *Error {
	return &Error{err, modelError, nil}
}

func NewDataError(msg string, err ...error) *Error {
	return &Error{
		error:   errors.New(msg),
		errType: dataError,
		wrapped: err,
	}
}

func (err *Error) IsDatabaseError() bool {
	return err.errType == databaseError
}

func (err *Error) IsModelError() bool {
	return err.errType == modelError
}

func (err *Error) IsDataError() bool {
	return err.errType == dataError
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	if wrapped, ok := e.error.(interface {
		Unwrap() error
	}); ok {
		if err := wrapped.Unwrap(); err != nil {
			return &Error{
				error:   err,
				errType: e.errType,
				wrapped: e.wrapped,
			}
		}
	}

	var err *Error
	if l := len(e.wrapped); l > 0 {
		err = &Error{
			error:   e.wrapped[0],
			errType: e.errType,
		}
		if l > 1 {
			err.wrapped = e.wrapped[1:]
		}
	}
	return err
}
