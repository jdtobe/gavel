package kevapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestResponseRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/response.json")
	if err != nil {
		t.Fatal(err)
	}
	var got Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.Model != ModelName {
		t.Fatalf("Model = %q", got.Model)
	}
	if got.Usage.InputTokens != 101 || got.Usage.OutputTokens != 161 {
		t.Fatalf("Usage = %#v", got.Usage)
	}
	if got.LatencyMS != 495 {
		t.Fatalf("LatencyMS = %v", got.LatencyMS)
	}

	choice, ok := got.Answers["department"]
	if !ok {
		t.Fatal("missing department answer")
	}
	if choice.Type != TypeChoice || choice.Choice != "returns" {
		t.Fatalf("choice = %#v", choice)
	}
	if choice.Confidence == nil || *choice.Confidence != 0.21 {
		t.Fatalf("choice confidence = %v", choice.Confidence)
	}
	if choice.Noul != nil || choice.Score != nil {
		t.Fatalf("choice should not carry noul/score: %#v", choice)
	}
	if got := choice.Probabilities["returns"]; got != 0.47 {
		t.Fatalf("returns probability = %v", got)
	}

	noul, ok := got.Answers["escalate"]
	if !ok {
		t.Fatal("missing escalate answer")
	}
	if noul.Type != TypeNoul || noul.Noul == nil || *noul.Noul != 0.93 {
		t.Fatalf("noul = %#v", noul)
	}
	if noul.Confidence != nil {
		t.Fatalf("noul should not carry confidence: %#v", noul)
	}

	score, ok := got.Answers["frustration"]
	if !ok {
		t.Fatal("missing frustration answer")
	}
	if score.Type != TypeScore || score.Score == nil || *score.Score != 1.44 {
		t.Fatalf("score = %#v", score)
	}
	if score.Confidence == nil || *score.Confidence != 0.34 {
		t.Fatalf("score confidence = %v", score.Confidence)
	}
	if score.Legend["2"] != "Very angry" {
		t.Fatalf("legend = %#v", score.Legend)
	}
	if got := score.Probabilities["0"]; got != 0 {
		t.Fatalf("zero probability = %v", got)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var round Response
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, round) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", round, got)
	}
}

func TestQuestionTypeValues(t *testing.T) {
	cases := map[QuestionType]string{
		TypeNoul:   "noul",
		TypeChoice: "choice",
		TypeScore:  "score",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Fatalf("%q != %q", got, want)
		}
	}
}

func TestResponsePreservesFractionalLatency(t *testing.T) {
	data := []byte(`{"model":"kev-latest","answers":{},"usage":{},"latency_ms":1754.9}`)
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["latency_ms"] != 1754.9 {
		t.Fatalf("fractional latency lost: %s", encoded)
	}
}

func TestRequestModelOmittedWhenEmpty(t *testing.T) {
	data, err := json.Marshal(Request{State: "x", Questions: map[string]Question{}})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["model"]; ok {
		t.Fatalf("model should be omitted: %s", data)
	}
}
