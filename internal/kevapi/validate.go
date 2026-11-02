package kevapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ValidationError describes a request field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateRequest mirrors Kev's request rules before anything is proxied.
func ValidateRequest(r Request) error {
	if strings.TrimSpace(r.State) == "" {
		return invalid("state", "state is required")
	}
	if len(r.Questions) == 0 {
		return invalid("questions", "at least one question is required")
	}

	ids := make([]string, 0, len(r.Questions))
	for id := range r.Questions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		q := r.Questions[id]
		if strings.TrimSpace(id) == "" {
			return invalid("questions", "question id must not be empty")
		}
		field := "questions." + id
		switch q.Type {
		case TypeNoul, TypeChoice, TypeScore:
		default:
			return invalid(field+".type", "unknown question type %q", q.Type)
		}
		if strings.TrimSpace(q.Instructions) == "" {
			return invalid(field+".instructions", "instructions are required")
		}
		var err error
		switch q.Type {
		case TypeNoul:
			err = validateNoul(q.Criteria)
		case TypeChoice:
			err = validateChoice(q.Criteria)
		case TypeScore:
			err = validateScore(q.Criteria)
		}
		if err != nil {
			return &ValidationError{Field: field + ".criteria", Message: err.Error()}
		}
	}
	return nil
}

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

func validateNoul(criteria json.RawMessage) error {
	trimmed := bytes.TrimSpace(criteria)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid criteria JSON: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("criteria must be an object")
	}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid criteria JSON: %w", err)
		}
		key, _ := keyTok.(string)
		if key != "true" && key != "false" {
			return fmt.Errorf("unknown noul criterion %q", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate noul criterion %q", key)
		}
		seen[key] = true
		var value *string
		if err := dec.Decode(&value); err != nil || value == nil {
			return fmt.Errorf("noul criterion %q must be a string", key)
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid criteria JSON: %w", err)
	}
	return nil
}

func validateChoice(criteria json.RawMessage) error {
	trimmed := bytes.TrimSpace(criteria)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("criteria must be an object with 2-255 options")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid criteria JSON: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("criteria must be an object")
	}
	seen := map[string]bool{}
	count := 0
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid criteria JSON: %w", err)
		}
		key, _ := keyTok.(string)
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("option name must not be empty")
		}
		if seen[key] {
			return fmt.Errorf("duplicate option %q", key)
		}
		seen[key] = true
		var description *string
		if err := dec.Decode(&description); err != nil {
			return fmt.Errorf("option %q description must be a string or null", key)
		}
		count++
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid criteria JSON: %w", err)
	}
	if count < 2 || count > 255 {
		return fmt.Errorf("need 2-255 options, got %d", count)
	}
	return nil
}

func validateScore(criteria json.RawMessage) error {
	var levels []string
	if err := json.Unmarshal(criteria, &levels); err != nil {
		return fmt.Errorf("criteria must be an array of strings")
	}
	if len(levels) < 2 || len(levels) > 255 {
		return fmt.Errorf("need 2-255 levels, got %d", len(levels))
	}
	for i, level := range levels {
		if strings.TrimSpace(level) == "" {
			return fmt.Errorf("level %d must not be empty", i)
		}
	}
	return nil
}
