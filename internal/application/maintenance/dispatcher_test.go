package maintenance

import (
	"errors"
	"testing"
)

func TestDispatchRunUsesMatchingExecutor(t *testing.T) {
	called := false
	err := DispatchRun(Run{RunType: RunTypeNodeObservation}, map[string]RunExecutor{
		RunTypeNodeObservation: func(Run) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("executor was not called")
	}
}

func TestDispatchRunRejectsUnknownType(t *testing.T) {
	err := DispatchRun(Run{RunType: "missing"}, nil)
	if !errors.Is(err, ErrUnknownRunType) {
		t.Fatalf("DispatchRun error = %v, want ErrUnknownRunType", err)
	}
}
