package recommendation

import (
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestGoldenSelection(t *testing.T) {
	var fixture struct {
		Description string           `json:"description"`
		Input       Input            `json:"input"`
		IDs         []string         `json:"expectedIDs"`
		Sources     [SourceCount]int `json:"expectedSources"`
		Relaxations []Relaxation     `json:"expectedRelaxations"`
		Features    Features         `json:"expectedFeatures"`
		More        bool             `json:"expectedHasMoreWithinInput"`
	}
	f, err := os.Open("testdata/selection-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err := d.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	r := selectOK(t, ExperimentalConfig(), fixture.Input)
	ids := make([]string, 0, len(r.Items))
	relaxations := make([]Relaxation, 0, len(r.Items))
	for _, item := range r.Items {
		ids = append(ids, item.ID)
		relaxations = append(relaxations, item.Relaxation)
		if item.Features != fixture.Features {
			t.Fatalf("features: %+v", item)
		}
	}
	if !reflect.DeepEqual(ids, fixture.IDs) || !reflect.DeepEqual(relaxations, fixture.Relaxations) || r.SourceCounts != fixture.Sources || r.HasMoreWithinInput != fixture.More {
		t.Fatalf("golden mismatch: %+v", r)
	}
}

func TestConcurrentCallsDoNotShareState(t *testing.T) {
	c := ExperimentalConfig()
	in := fixture(40)
	want := selectOK(t, c, in)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := Select(c, in)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("concurrent selection changed: %v", err)
			}
		}()
	}
	wg.Wait()
}
