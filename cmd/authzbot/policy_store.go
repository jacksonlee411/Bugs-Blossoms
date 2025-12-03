package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	jsondiff "github.com/wI2L/jsondiff"
)

type policyStore struct {
	dir          string
	doc          map[string][][]string
	entryFiles   map[string]string
	typeDefaults map[string]string
	existing     map[string]struct{}
}

func loadPolicyStore(dir string) (*policyStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	store := &policyStore{
		dir:          dir,
		doc:          map[string][][]string{},
		entryFiles:   map[string]string{},
		typeDefaults: map[string]string{},
		existing:     map[string]struct{}{},
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".csv" {
			return nil
		}
		if err := store.loadFile(path); err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		store.existing[path] = struct{}{}
		return nil
	}); err != nil {
		return nil, err
	}
	ensureDocKeys(store.doc, "p", "g")
	return store, nil
}

func ensureDocKeys(doc map[string][][]string, keys ...string) {
	for _, k := range keys {
		if _, ok := doc[k]; !ok {
			doc[k] = [][]string{}
		}
	}
}

func (s *policyStore) loadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		record, err := parsePolicyLine(line)
		if err != nil {
			return err
		}
		if len(record) == 0 {
			continue
		}
		typ := record[0]
		values := append([]string(nil), record[1:]...)
		s.doc[typ] = append(s.doc[typ], values)
		key := policyEntryKey(typ, values)
		s.entryFiles[key] = path
		if _, ok := s.typeDefaults[typ]; !ok {
			s.typeDefaults[typ] = path
		}
	}
	return scanner.Err()
}

func parsePolicyLine(line string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(line))
	reader.TrimLeadingSpace = true
	rec, err := reader.Read()
	if err != nil {
		return nil, err
	}
	for i := range rec {
		rec[i] = strings.TrimSpace(rec[i])
	}
	return rec, nil
}

func (s *policyStore) Apply(diff json.RawMessage) (json.RawMessage, error) {
	before, err := json.Marshal(s.doc)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch(diff)
	if err != nil {
		return nil, err
	}
	afterBytes, err := patch.Apply(before)
	if err != nil {
		return nil, err
	}
	updated := map[string][][]string{}
	if err := json.Unmarshal(afterBytes, &updated); err != nil {
		return nil, err
	}
	ensureDocKeys(updated, "p", "g")
	if err := s.write(updated); err != nil {
		return nil, err
	}
	reversePatch, err := jsondiff.CompareJSON(afterBytes, before)
	if err != nil {
		return nil, err
	}
	return json.Marshal(reversePatch)
}

func (s *policyStore) write(doc map[string][][]string) error {
	files := map[string][]policyEntry{}
	for typ, rows := range doc {
		for _, values := range rows {
			entry := policyEntry{Type: typ, Values: values}
			path := s.resolveFile(entry)
			files[path] = append(files[path], entry)
		}
	}

	for path, entries := range files {
		if err := s.writeFile(path, entries); err != nil {
			return err
		}
	}
	for path := range s.existing {
		if _, ok := files[path]; !ok {
			_ = os.Remove(path)
		}
	}
	s.doc = doc
	s.existing = map[string]struct{}{}
	for path := range files {
		s.existing[path] = struct{}{}
	}
	return nil
}

func (s *policyStore) resolveFile(entry policyEntry) string {
	key := policyEntryKey(entry.Type, entry.Values)
	if path, ok := s.entryFiles[key]; ok {
		return path
	}
	if path, ok := s.typeDefaults[entry.Type]; ok {
		return path
	}
	fallback := filepath.Join(s.dir, "core", "global.csv")
	if _, err := os.Stat(filepath.Dir(fallback)); errors.Is(err, os.ErrNotExist) {
		_ = os.MkdirAll(filepath.Dir(fallback), 0o755)
	}
	s.typeDefaults[entry.Type] = fallback
	return fallback
}

func (s *policyStore) writeFile(path string, entries []policyEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	sortEntries(entries)
	builder := &strings.Builder{}
	for _, entry := range entries {
		fields := append([]string{entry.Type}, entry.Values...)
		builder.WriteString(strings.Join(fields, ", "))
		builder.WriteString("\n")
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

type policyEntry struct {
	Type   string
	Values []string
}

func sortEntries(entries []policyEntry) {
	sort.Slice(entries, func(i, j int) bool {
		ai := append([]string{entries[i].Type}, entries[i].Values...)
		aj := append([]string{entries[j].Type}, entries[j].Values...)
		return strings.Join(ai, "|") < strings.Join(aj, "|")
	})
}

func policyEntryKey(typ string, values []string) string {
	return fmt.Sprintf("%s|%s", typ, strings.Join(values, "\x1f"))
}
