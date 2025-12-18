package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

func openCSV(path string) (*csv.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	br := bufio.NewReader(f)
	br = stripUTF8BOM(br)

	r := csv.NewReader(br)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = false
	return r, f.Close, nil
}

func stripUTF8BOM(r *bufio.Reader) *bufio.Reader {
	b, err := r.Peek(3)
	if err == nil && len(b) == 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = r.Discard(3)
	}
	return r
}

func readHeader(r *csv.Reader) ([]string, error) {
	h, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("missing header")
		}
		return nil, err
	}
	for i := range h {
		h[i] = strings.TrimSpace(h[i])
		if !utf8.ValidString(h[i]) {
			return nil, fmt.Errorf("invalid header encoding")
		}
	}
	return h, nil
}

func headerIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, name := range header {
		m[name] = i
	}
	return m
}

func requireHeader(header []string, required []string, allowed []string) error {
	hset := make(map[string]struct{}, len(header))
	for _, h := range header {
		hset[h] = struct{}{}
	}
	for _, req := range required {
		if _, ok := hset[req]; !ok {
			return fmt.Errorf("missing required header column: %s", req)
		}
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	for _, h := range header {
		if _, ok := allowedSet[h]; !ok {
			return fmt.Errorf("unexpected header column: %s", h)
		}
	}
	return nil
}
