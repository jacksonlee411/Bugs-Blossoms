package person

import "errors"

var (
	ErrNotFound   = errors.New("person not found")
	ErrPernrTaken = errors.New("person pernr already exists")
)
