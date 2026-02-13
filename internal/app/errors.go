package app

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	KindUsage        ErrorKind = "usage"
	KindUnauthorized ErrorKind = "unauthorized"
	KindTransient    ErrorKind = "transient"
	KindInternal     ErrorKind = "internal"
)

type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Wrap(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Op: op, Err: err}
}

func KindOf(err error) ErrorKind {
	if err == nil {
		return ""
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return KindInternal
}
