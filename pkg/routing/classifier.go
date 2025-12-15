package routing

import (
	"regexp"
	"sort"
	"strings"
)

var internalAPIPrefixPattern = regexp.MustCompile(`^/[^/]+/api(?:/|$)`)

type Classifier struct {
	rules []AllowlistRule
}

func NewClassifier(rules []AllowlistRule) *Classifier {
	copied := make([]AllowlistRule, 0, len(rules))
	for _, rule := range rules {
		rule.Prefix = strings.TrimSpace(rule.Prefix)
		if rule.Prefix == "" {
			continue
		}
		copied = append(copied, rule)
	}

	sort.SliceStable(copied, func(i, j int) bool {
		return len(copied[i].Prefix) > len(copied[j].Prefix)
	})

	return &Classifier{
		rules: copied,
	}
}

func (c *Classifier) MatchAllowlist(path string) (RouteClass, bool) {
	for _, rule := range c.rules {
		if HasPathPrefixOnBoundary(path, rule.Prefix) {
			return rule.Class, true
		}
	}
	return "", false
}

func (c *Classifier) ClassifyPath(path string) RouteClass {
	if class, ok := c.MatchAllowlist(path); ok {
		return class
	}

	if HasPathPrefixOnBoundary(path, "/api/v1") {
		return RouteClassPublicAPI
	}
	if internalAPIPrefixPattern.MatchString(path) {
		return RouteClassInternalAPI
	}
	return RouteClassUI
}

func HasPathPrefixOnBoundary(path, prefix string) bool {
	if prefix == "" {
		return false
	}

	if prefix == "/" {
		return strings.HasPrefix(path, "/")
	}

	if !strings.HasPrefix(path, prefix) {
		return false
	}

	if len(path) == len(prefix) {
		return true
	}

	if strings.HasSuffix(prefix, "/") {
		return true
	}

	return path[len(prefix)] == '/'
}
