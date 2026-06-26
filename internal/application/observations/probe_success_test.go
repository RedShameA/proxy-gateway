package observations

import "testing"

func TestParseProbeResponseSupportsJSONPayload(t *testing.T) {
	result := ParseProbeResponse([]byte(`{"ip":"203.0.113.44","country":"jp"}`))
	if result.EgressIP != "203.0.113.44" || result.ProbeCountry != "JP" {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseProbeResponseSupportsTracePayload(t *testing.T) {
	result := ParseProbeResponse([]byte("ip=198.51.100.20\nloc=sg\n"))
	if result.EgressIP != "198.51.100.20" || result.ProbeCountry != "SG" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildSuccessRecordPrefersLocalGeoIPCountry(t *testing.T) {
	record := BuildSuccessRecord(ParseProbeResponse([]byte("ip=198.51.100.20\nloc=sg\n")), "US", 37)
	if record.EgressIP != "198.51.100.20" || record.EgressCountry != "US" || record.LatencyMS != 37 {
		t.Fatalf("record = %#v", record)
	}
}

func TestBuildSuccessRecordFallsBackToProbeCountry(t *testing.T) {
	record := BuildSuccessRecord(ParseProbeResponse([]byte(`{"ip":"203.0.113.44","country":"jp"}`)), "", 19)
	if record.EgressIP != "203.0.113.44" || record.EgressCountry != "JP" || record.LatencyMS != 19 {
		t.Fatalf("record = %#v", record)
	}
}
