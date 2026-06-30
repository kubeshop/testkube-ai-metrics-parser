package naming

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Answer Relevancy":           "answer_relevancy",
		"exact_match,strict-match":   "exact_match_strict_match",
		"  Faithfulness!!  ":         "faithfulness",
		"already_normal":             "already_normal",
		"GPT-4o":                     "gpt_4o",
		"":                           "",
		"---":                        "",
		"multiple   spaces and-dash": "multiple_spaces_and_dash",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
