package services

import "errors"

var (
	ErrRevisionMismatch = errors.New("policy base revision is stale")
	ErrPolicyApply      = errors.New("policy apply failed")
)
