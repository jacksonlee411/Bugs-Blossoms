package main

import "errors"

// keep error handling in one place (avoids importing errors in every file).
func as(err error, target any) bool { return errors.As(err, target) }
