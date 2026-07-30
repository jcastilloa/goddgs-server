package chrome

import "errors"

type classifiedError struct {
	kind  error
	cause error
}

func (e classifiedError) Error() string {
	return e.kind.Error()
}

func (e classifiedError) Unwrap() []error {
	return []error{e.kind, e.cause}
}

func classifyError(kind, cause error) error {
	if cause == nil || errors.Is(cause, kind) {
		return kind
	}
	return classifiedError{kind: kind, cause: cause}
}
