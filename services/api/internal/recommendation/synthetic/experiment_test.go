package synthetic

import (
	"reflect"
	"testing"
)

func TestExperimentReproducibleAndSmallPoolRecyclesOnlyUnexposed(t *testing.T) {
	a, err := Run()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Cases) != 30 || !reflect.DeepEqual(a, b) {
		t.Fatal("unstable finite experiment")
	}
	for _, c := range a.Cases {
		if !c.RetryIdentical || len(c.ConfigSHA256) != 64 || len(c.InputSHA256) != 64 {
			t.Fatal(c)
		}
		if c.Scenario == "small-7" {
			if c.First.Selected != 7 || !c.CandidateShortfall || c.NextNewRound.Selected != 5 || c.NextNewRound.DeliveryRecycled != 5 || c.AdjacentOverlap != 5 {
				t.Fatal(c)
			}
		} else if c.AdjacentOverlap != 0 || c.NextNewRound.Selected != 10 {
			t.Fatal("sufficient unseen pool unexpectedly overlapped", c)
		}
	}
}
