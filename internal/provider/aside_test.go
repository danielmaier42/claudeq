package provider

import (
	"encoding/json"
	"testing"
)

func TestAsideJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Aside
		want string
	}{
		{"validated wins", Aside{Text: `{"a":2}`, Structured: json.RawMessage(`{"a":1}`)}, `{"a":1}`},
		{"bare object", Aside{Text: `{"a":1}`}, `{"a":1}`},
		{"code fence", Aside{Text: "```json\n{\"a\":{\"b\":1}}\n```"}, `{"a":{"b":1}}`},
		{"sentence around it", Aside{Text: `Here it is: {"a":1} Hope that helps.`}, `{"a":1}`},
		{"braces in the prose", Aside{Text: `Use {placeholders}: {"a":1}. Or {this}.`}, `{"a":1}`},
		{"two objects", Aside{Text: `{"a":1} and {"b":2}`}, `{"a":1}`},
		{"null structured falls back to the text", Aside{Text: `{"a":1}`, Structured: json.RawMessage(`null`)}, `{"a":1}`},
		{"no object", Aside{Text: "just prose"}, ""},
		{"only a closing brace", Aside{Text: "} {"}, ""},
	} {
		if got := string(tc.in.JSON()); got != tc.want {
			t.Errorf("%s: JSON() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
