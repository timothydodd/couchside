// Package usererr marks errors whose text is written for the person using
// Couchside ("all tuners are busy"), so the API may show it. Any other error
// that reaches a 500 is logged and answered "internal error", since its text
// may hold paths, SQL or addresses.
package usererr

import (
	"errors"
	"fmt"
)

type userError struct{ msg string }

func (e *userError) Error() string { return e.msg }

// New is an error whose message can be shown to users.
func New(msg string) error { return &userError{msg} }

// Errorf is New with formatting. Wrapped errors (%w) aren't kept: the result
// is only a message.
func Errorf(format string, a ...any) error { return &userError{fmt.Sprintf(format, a...)} }

// Is reports whether err (or one it wraps) is meant for users.
func Is(err error) bool {
	var u *userError
	return errors.As(err, &u)
}
