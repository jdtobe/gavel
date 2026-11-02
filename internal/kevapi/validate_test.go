package kevapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func raw(t *testing.T, s string) json.RawMessage {
	t.Helper()
	if !json.Valid([]byte(s)) {
		t.Fatalf("test fixture is not valid JSON: %s", s)
	}
	return json.RawMessage(s)
}

func withQuestion(q Question) Request {
	return Request{State: "ticket", Questions: map[string]Question{"q1": q}}
}

func manyChoice(t *testing.T, n int) json.RawMessage {
	t.Helper()
	m := make(map[string]any, n)
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("o%d", i)] = nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func manyLevels(t *testing.T, n int) json.RawMessage {
	t.Helper()
	levels := make([]string, n)
	for i := range levels {
		levels[i] = fmt.Sprintf("level %d", i)
	}
	b, err := json.Marshal(levels)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateRejectsEmptyState(t *testing.T) {
	err := ValidateRequest(Request{State: "   ", Questions: map[string]Question{
		"q1": {Type: TypeNoul, Instructions: "Is it true?"},
	}})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "state" {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateAcceptsValidRequests(t *testing.T) {
	cases := []struct {
		name string
		req  Request
	}{
		{
			name: "all three types",
			req: Request{State: "ticket", Questions: map[string]Question{
				"department":  {Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{"returns":"refunds","billing":null}`)},
				"escalate":    {Type: TypeNoul, Instructions: "urgent?"},
				"frustration": {Type: TypeScore, Instructions: "how frustrated?", Criteria: raw(t, `["Calm","Frustrated"]`)},
			}},
		},
		{
			name: "noul with descriptions",
			req:  withQuestion(Question{Type: TypeNoul, Instructions: "urgent?", Criteria: raw(t, `{"true":"yes","false":"no"}`)}),
		},
		{
			name: "noul with empty criteria object",
			req:  withQuestion(Question{Type: TypeNoul, Instructions: "urgent?", Criteria: raw(t, `{}`)}),
		},
		{
			name: "choice at lower bound",
			req:  withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{"a":null,"b":"B"}`)}),
		},
		{
			name: "choice at upper bound",
			req:  withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: manyChoice(t, 255)}),
		},
		{
			name: "score at lower bound",
			req:  withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: raw(t, `["low","high"]`)}),
		},
		{
			name: "score at upper bound",
			req:  withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: manyLevels(t, 255)}),
		},
		{
			name: "instructions are trimmed",
			req:  withQuestion(Question{Type: TypeNoul, Instructions: "  urgent?  "}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateRequest(tc.req); err != nil {
				t.Fatalf("ValidateRequest() = %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name  string
		req   Request
		field string
	}{
		{
			name:  "empty state",
			req:   Request{State: " \n\t", Questions: map[string]Question{"q1": {Type: TypeNoul, Instructions: "x"}}},
			field: "state",
		},
		{
			name:  "zero questions",
			req:   Request{State: "ticket"},
			field: "questions",
		},
		{
			name:  "empty question id",
			req:   Request{State: "ticket", Questions: map[string]Question{"": {Type: TypeNoul, Instructions: "x"}}},
			field: "questions",
		},
		{
			name:  "unknown type",
			req:   withQuestion(Question{Type: "bogus", Instructions: "x"}),
			field: "questions.q1.type",
		},
		{
			name:  "empty instructions",
			req:   withQuestion(Question{Type: TypeNoul, Instructions: "  "}),
			field: "questions.q1.instructions",
		},
		{
			name:  "choice zero options",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice one option",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{"a":null}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice 256 options",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: manyChoice(t, 256)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice empty option name",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{"":null,"b":null}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice duplicate option name",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{"a":null,"a":"again"}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice numeric description",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `{"a":1,"b":null}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice criteria not an object",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick", Criteria: raw(t, `["a","b"]`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "choice missing criteria",
			req:   withQuestion(Question{Type: TypeChoice, Instructions: "pick"}),
			field: "questions.q1.criteria",
		},
		{
			name:  "score zero levels",
			req:   withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: raw(t, `[]`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "score one level",
			req:   withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: raw(t, `["a"]`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "score 256 levels",
			req:   withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: manyLevels(t, 256)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "score empty level",
			req:   withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: raw(t, `["a",""]`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "score non-string level",
			req:   withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: raw(t, `["a",1]`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "score criteria not an array",
			req:   withQuestion(Question{Type: TypeScore, Instructions: "scale", Criteria: raw(t, `{}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "noul criteria not an object",
			req:   withQuestion(Question{Type: TypeNoul, Instructions: "x", Criteria: raw(t, `[]`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "noul true description not a string",
			req:   withQuestion(Question{Type: TypeNoul, Instructions: "x", Criteria: raw(t, `{"true":1}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "noul false description not a string",
			req:   withQuestion(Question{Type: TypeNoul, Instructions: "x", Criteria: raw(t, `{"false":false}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "noul null description",
			req:   withQuestion(Question{Type: TypeNoul, Instructions: "x", Criteria: raw(t, `{"true":null}`)}),
			field: "questions.q1.criteria",
		},
		{
			name:  "malformed criteria",
			req:   withQuestion(Question{Type: TypeNoul, Instructions: "x", Criteria: json.RawMessage(`{`)}),
			field: "questions.q1.criteria",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRequest(tc.req)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if ve.Field != tc.field {
				t.Fatalf("field = %q, want %q (message %q)", ve.Field, tc.field, ve.Message)
			}
			if ve.Message == "" {
				t.Fatal("message is empty")
			}
		})
	}
}

func TestValidationErrorString(t *testing.T) {
	ve := &ValidationError{Field: "questions.q1.criteria", Message: "need at least 2 options"}
	got := ve.Error()
	if !strings.Contains(got, ve.Field) || !strings.Contains(got, ve.Message) {
		t.Fatalf("Error() = %q", got)
	}
}
