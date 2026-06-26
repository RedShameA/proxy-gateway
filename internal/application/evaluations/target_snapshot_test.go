package evaluations

import "testing"

func TestBuildTargetSnapshotUsesDefaultTestURLAndFallsBackExitNodesToFixedNode(t *testing.T) {
	snapshot, skipped := BuildTargetSnapshot(TargetSnapshotInput{
		FixedNodeID:                         "node-fixed",
		TestURL:                             "",
		DefaultTestURL:                      "https://www.gstatic.com/generate_204",
		DefaultMinEvaluationIntervalSeconds: 300,
	})

	if skipped {
		t.Fatalf("skipped = true, want false")
	}
	if snapshot.TestURL != "https://www.gstatic.com/generate_204" {
		t.Fatalf("TestURL = %q", snapshot.TestURL)
	}
	if len(snapshot.ExitNodeIDs) != 1 || snapshot.ExitNodeIDs[0] != "node-fixed" {
		t.Fatalf("ExitNodeIDs = %#v", snapshot.ExitNodeIDs)
	}
}

func TestBuildTargetSnapshotSkipsWhenDefaultMinIntervalHasNotElapsed(t *testing.T) {
	_, skipped := BuildTargetSnapshot(TargetSnapshotInput{
		LastEvaluatedAt:                     1_000,
		DefaultMinEvaluationIntervalSeconds: 300,
		NowMS:                               1_000 + 299_000,
	})

	if !skipped {
		t.Fatal("skipped = false, want true")
	}
}
