// Package ownership reads the ownership rules shared by CODEOWNERS generation and toolset triage.
package ownership

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const InputPath = ".github/ownership.json"

type Area struct {
	Toolset   string   `json:"toolset"`
	Paths     []string `json:"paths"`
	Approvers []string `json:"approvers"`
	Reviewers []string `json:"reviewers"`
}

type Config struct {
	DefaultApprovers []string `json:"default_approvers"`
	DefaultReviewers []string `json:"default_reviewers"`
	Areas            []Area   `json:"areas"`
}

var (
	toolsetName = regexp.MustCompile(`^[a-z0-9_-]+(/[a-z0-9_-]+)?$`)
	githubUser  = regexp.MustCompile(`^@[A-Za-z0-9][A-Za-z0-9-]*(/[A-Za-z0-9][A-Za-z0-9_-]*)?$`)
)

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, fmt.Errorf("parse %s: expected one JSON document, got %v", path, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", path, err)
	}
	return config, nil
}

func (config Config) Validate() error {
	if len(config.DefaultApprovers) == 0 {
		return fmt.Errorf("default_approvers must not be empty")
	}
	if err := validateUsers(config.DefaultApprovers); err != nil {
		return err
	}
	if err := validateUsers(config.DefaultReviewers); err != nil {
		return err
	}
	seenToolsets := make(map[string]bool)
	var paths []string
	for _, area := range config.Areas {
		if len(area.Paths) == 0 {
			return fmt.Errorf("area must have at least one path")
		}
		if area.Toolset != "" {
			if !toolsetName.MatchString(area.Toolset) || seenToolsets[area.Toolset] {
				return fmt.Errorf("invalid or duplicate toolset %q", area.Toolset)
			}
			seenToolsets[area.Toolset] = true
		}
		if err := validateUsers(area.Approvers); err != nil {
			return err
		}
		if err := validateUsers(area.Reviewers); err != nil {
			return err
		}
		for _, path := range area.Paths {
			if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, " \t\r\n#") ||
				(strings.Contains(path, "*") && (!strings.HasSuffix(path, "*") || strings.Count(path, "*") != 1)) {
				return fmt.Errorf("invalid CODEOWNERS path %q", path)
			}
			// CODEOWNERS uses the last matching rule, while triage matches every
			// area. Different overlapping patterns would make one area lose its
			// approvers depending on the order of the generated sections.
			for _, previous := range paths {
				if previous != path && (covers(previous, path) || covers(path, previous)) {
					return fmt.Errorf("overlapping CODEOWNERS paths %q and %q", previous, path)
				}
			}
			paths = append(paths, path)
		}
	}
	return nil
}

func covers(pattern, path string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(path, pattern)
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func validateUsers(users []string) error {
	for _, user := range users {
		if !githubUser.MatchString(user) {
			return fmt.Errorf("invalid GitHub user or team %q", user)
		}
	}
	return nil
}

func Unique(users ...[]string) []string {
	unique := []string{}
	seen := make(map[string]bool)
	for _, group := range users {
		for _, user := range group {
			key := strings.ToLower(user)
			if !seen[key] {
				unique = append(unique, user)
				seen[key] = true
			}
		}
	}
	return unique
}
