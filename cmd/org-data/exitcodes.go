package main

type cliError struct {
	code int
	err  error
}

func (e *cliError) Error() string {
	return e.err.Error()
}

func (e *cliError) Unwrap() error {
	return e.err
}

const (
	exitOK         = 0
	exitValidation = 2
	exitUsage      = 3
	exitDB         = 4
	exitDBWrite    = 5
	exitSafetyNet  = 6
)

func withCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &cliError{code: code, err: err}
}

func exitCode(err error) int {
	if err == nil {
		return exitOK
	}
	var ce *cliError
	if ok := as(err, &ce); ok {
		return ce.code
	}
	return 1
}
