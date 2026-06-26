package subscriptions

import "testing"

func TestBuildRefreshFailureOutcome(t *testing.T) {
	fetch := BuildRefreshFetchFailure("sub-1")
	if fetch.SubscriptionID != "sub-1" || fetch.ReasonCode != "fetch_failed" || !fetch.PersistLastError {
		t.Fatalf("fetch outcome = %#v", fetch)
	}

	parse := BuildRefreshImportFailure("sub-2", true)
	if parse.SubscriptionID != "sub-2" || parse.ReasonCode != "parse_failed" || !parse.PersistLastError {
		t.Fatalf("parse outcome = %#v", parse)
	}

	importFailure := BuildRefreshImportFailure("sub-3", false)
	if importFailure.SubscriptionID != "sub-3" || importFailure.ReasonCode != "import_failed" || !importFailure.PersistLastError {
		t.Fatalf("import failure outcome = %#v", importFailure)
	}
}
