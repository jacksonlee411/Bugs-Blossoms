package commands

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/commands/common"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
)

type trUsage struct {
	Key  string
	File string
	Line int
}

var templTCallRe = regexp.MustCompile(`\.T\("([^"]+)"\)`)

func CheckTrUsage(allowedLanguages []string, mods ...application.Module) error {
	if len(allowedLanguages) == 0 {
		allowedLanguages = []string{"en", "zh"}
	}

	conf := configuration.Use()
	app, pool, err := common.NewApplicationWithDefaults(mods...)
	if err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}
	defer pool.Close()

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	usages, err := collectTrUsages(root)
	if err != nil {
		return err
	}

	if len(usages) == 0 {
		return fmt.Errorf("no translation usages found")
	}

	messages := app.Bundle().Messages()

	allowed := make(map[string]language.Tag, len(allowedLanguages))
	for _, code := range allowedLanguages {
		tag, err := language.Parse(code)
		if err != nil {
			return fmt.Errorf("invalid allowed language %q: %w", code, err)
		}
		allowed[code] = tag
		if messages[tag] == nil {
			return fmt.Errorf("allowed language %q (%s) not found in bundle", code, tag)
		}
	}

	type missingKey struct {
		Locale string
		Key    string
		File   string
		Line   int
	}

	var missing []missingKey
	seen := make(map[string]bool)
	for _, u := range usages {
		if u.Key == "" {
			continue
		}
		// Validate each unique key once; keep the first occurrence for reporting.
		if seen[u.Key] {
			continue
		}
		seen[u.Key] = true

		for locale, tag := range allowed {
			if messages[tag][u.Key] == nil {
				missing = append(missing, missingKey{
					Locale: locale,
					Key:    u.Key,
					File:   u.File,
					Line:   u.Line,
				})
			}
		}
	}

	if len(missing) > 0 {
		for _, m := range missing {
			conf.Logger().WithFields(logrus.Fields{
				"locale": m.Locale,
				"key":    m.Key,
				"source": fmt.Sprintf("%s:%d", m.File, m.Line),
			}).Error("Translation key missing in allowed locales")
		}
		return fmt.Errorf("some translation keys are missing in allowed locales")
	}

	conf.Logger().WithFields(logrus.Fields{
		"allowed_locales": strings.Join(allowedLanguages, ", "),
		"unique_keys":     len(seen),
	}).Info("All translation usages are present in allowed locales")

	return nil
}

func collectTrUsages(root string) ([]trUsage, error) {
	var usages []trUsage

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") {
				return fs.SkipDir
			}
			if rel == "vendor" || strings.HasPrefix(rel, "vendor/") {
				return fs.SkipDir
			}
			if rel == "node_modules" || strings.HasPrefix(rel, "node_modules/") {
				return fs.SkipDir
			}
			if rel == "e2e/node_modules" || strings.HasPrefix(rel, "e2e/node_modules/") {
				return fs.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(rel, "_templ.go") {
			return nil
		}

		switch {
		case strings.HasSuffix(rel, ".go"):
			fileUsages, err := collectTrUsagesFromGoFile(path, rel)
			if err != nil {
				return err
			}
			usages = append(usages, fileUsages...)
		case strings.HasSuffix(rel, ".templ"):
			fileUsages, err := collectTrUsagesFromTemplFile(path, rel)
			if err != nil {
				return err
			}
			usages = append(usages, fileUsages...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return usages, nil
}

func collectTrUsagesFromTemplFile(absPath, relPath string) ([]trUsage, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var usages []trUsage
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		matches := templTCallRe.FindAllStringSubmatchIndex(text, -1)
		for _, m := range matches {
			if len(m) < 4 {
				continue
			}
			key := text[m[2]:m[3]]
			usages = append(usages, trUsage{
				Key:  key,
				File: relPath,
				Line: line,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return usages, nil
}

func collectTrUsagesFromGoFile(absPath, relPath string) ([]trUsage, error) {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, src, 0)
	if err != nil {
		return nil, err
	}

	var usages []trUsage
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			selector, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "T":
				if len(node.Args) < 1 {
					return true
				}
				if key, ok := stringLiteral(node.Args[0]); ok {
					pos := fset.Position(node.Args[0].Pos())
					usages = append(usages, trUsage{Key: key, File: relPath, Line: pos.Line})
				}
			case "MustT":
				if len(node.Args) < 2 {
					return true
				}
				if key, ok := stringLiteral(node.Args[1]); ok {
					pos := fset.Position(node.Args[1].Pos())
					usages = append(usages, trUsage{Key: key, File: relPath, Line: pos.Line})
				}
			}
		case *ast.CompositeLit:
			for _, elt := range node.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				keyIdent, ok := kv.Key.(*ast.Ident)
				if !ok || keyIdent.Name != "MessageID" {
					continue
				}
				msgID, ok := stringLiteral(kv.Value)
				if !ok {
					continue
				}
				pos := fset.Position(kv.Value.Pos())
				usages = append(usages, trUsage{Key: msgID, File: relPath, Line: pos.Line})
			}
		}
		return true
	})

	return usages, nil
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}
