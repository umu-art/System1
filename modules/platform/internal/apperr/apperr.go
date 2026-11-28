package apperr

import "errors"

// Kind — вид ошибки, по нему общий обработчик выбирает HTTP-код.
type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthenticated
	KindForbidden
	KindNotFound
	KindConflict
	KindNotImplemented
)

// Error — ошибка, которую service возвращает наверх, Message уходит клиенту.
type Error struct {
	Kind    Kind
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Message + ": " + e.cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.cause
}

// Wrap создаёт ошибку вида kind и сохраняет исходную ошибку для лога.
func Wrap(kind Kind, message string, cause error) error {
	return &Error{Kind: kind, Message: message, cause: cause}
}

func Invalid(message string) error {
	return &Error{Kind: KindInvalid, Message: message}
}

func Unauthenticated(message string) error {
	return &Error{Kind: KindUnauthenticated, Message: message}
}

func Forbidden(message string) error {
	return &Error{Kind: KindForbidden, Message: message}
}

func NotFound(message string) error {
	return &Error{Kind: KindNotFound, Message: message}
}

func Conflict(message string) error {
	return &Error{Kind: KindConflict, Message: message}
}

func NotImplemented() error {
	return &Error{Kind: KindNotImplemented, Message: "not implemented"}
}

// As достаёт *Error из цепочки ошибок.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
