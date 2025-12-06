package models

import "time"

type AuthenticationLog struct {
	ID        uint
	TenantID  string
	UserID    uint
	IP        string
	UserAgent string
	CreatedAt time.Time
}

type ActionLog struct {
	ID        uint
	TenantID  string
	UserID    *uint
	Method    string
	Path      string
	Before    []byte
	After     []byte
	UserAgent string
	IP        string
	CreatedAt time.Time
}
