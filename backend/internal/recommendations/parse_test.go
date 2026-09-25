package recommendations

import "testing"

func TestParseRecommendationsShapes(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want int
	}{
		"wrapped":      {`{"recommendations":[{"title":"A"},{"title":"B"}]}`, 2},
		"items key":    {`{"items":[{"title":"A"}]}`, 1},
		"bare array":   {`[{"title":"A"},{"title":"B"},{"title":"C"}]`, 3},
		"single":       {`{"source":"search_visibility","title":"A {with braces}"}`, 1},
		"back to back": {"{\"title\":\"A\"},\n{\"title\":\"B\"}\n{\"title\":\"C\"}", 3},
		"fenced":       {"```json\n{\"recommendations\":[{\"title\":\"A\"}]}\n```", 1},
		"prose":        {`Here you go: {"recommendations":[{"title":"A"}]} thanks`, 1},
		"empty":        {`{}`, 0},
	}
	for name, c := range cases {
		if got := len(parseRecommendations(c.raw)); got != c.want {
			t.Errorf("%s: got %d items, want %d", name, got, c.want)
		}
	}
}
