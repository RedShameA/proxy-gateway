package readmodel

import "testing"

func TestParseJSONObjectReturnsEmptyMapForInvalidOrEmptyInput(t *testing.T) {
	for _, raw := range []string{"", "[]", "not json"} {
		if got := ParseJSONObject(raw); got == nil || len(got) != 0 {
			t.Fatalf("ParseJSONObject(%q) = %#v, want empty map", raw, got)
		}
	}
}

func TestParseJSONObjectParsesObject(t *testing.T) {
	got := ParseJSONObject(`{"ok":true}`)
	if got["ok"] != true {
		t.Fatalf("ParseJSONObject = %#v, want ok=true", got)
	}
}
