package lineprotocol

import "testing"

func TestSerializeSortsAndFormats(t *testing.T) {
	records := []Record{
		{
			Measurement: "deepeval",
			Tags:        map[string]string{"metric": "answer_relevancy", "model": "gpt-4o"},
			Fields: map[string]Value{
				"mean_score": Float(0.85),
				"passed":     Int(2),
			},
		},
	}
	got := Serialize(records)
	want := "deepeval,metric=answer_relevancy,model=gpt-4o mean_score=0.85,passed=2i\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSerializeMergesIdentityAndSumsFields(t *testing.T) {
	records := []Record{
		{Measurement: "m", Tags: map[string]string{"t": "a"}, Fields: map[string]Value{"x": Int(1)}},
		{Measurement: "m", Tags: map[string]string{"t": "a"}, Fields: map[string]Value{"x": Int(2), "y": Float(3)}},
	}
	got := Serialize(records)
	want := "m,t=a x=3,y=3\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSerializeEscapesSpecialCharacters(t *testing.T) {
	records := []Record{
		{Measurement: "my eval", Tags: map[string]string{"k e y": "v,al"}, Fields: map[string]Value{"f": Uint(7)}},
	}
	got := Serialize(records)
	want := `my\ eval,k\ e\ y=v\,al f=7u` + "\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSerializeSkipsRecordsWithoutFields(t *testing.T) {
	if got := Serialize([]Record{{Measurement: "m"}}); got != "" {
		t.Errorf("expected empty output, got %q", got)
	}
}
