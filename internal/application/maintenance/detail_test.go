package maintenance

import "testing"

func TestRunDetailReturnsNonNilMap(t *testing.T) {
	detail := RunDetail(Run{})
	if detail == nil || len(detail) != 0 {
		t.Fatalf("detail = %#v, want empty non-nil map", detail)
	}
}

func TestDetailNumberHelpersAcceptDatabaseJSONTypes(t *testing.T) {
	if got := Int64FromDetail(float64(7)); got != 7 {
		t.Fatalf("Int64FromDetail = %d, want 7", got)
	}
	if got := IntFromDetail(int64(9), 0); got != 9 {
		t.Fatalf("IntFromDetail = %d, want 9", got)
	}
	if got := IntFromDetail("bad", 3); got != 3 {
		t.Fatalf("IntFromDetail fallback = %d, want 3", got)
	}
}
