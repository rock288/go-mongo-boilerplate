package main

import (
	"reflect"
	"testing"
)

func TestFindFeature_Known(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"kafka", "sqs", "observability", "samples"} {
		if findFeature(name) == nil {
			t.Errorf("findFeature(%q) = nil, want entry", name)
		}
	}
}

func TestFindFeature_Unknown(t *testing.T) {
	t.Parallel()
	if findFeature("nope") != nil {
		t.Fatal("expected nil for unknown feature")
	}
}

func TestComputeStripList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		nk, ns, no, nsa bool
		want            []string
	}{
		{"all-off", false, false, false, false, nil},
		{"only-kafka", true, false, false, false, []string{"kafka"}},
		{"kafka-obs", true, false, true, false, []string{"kafka", "observability"}},
		{"all-on", true, true, true, true, []string{"kafka", "sqs", "observability", "samples"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeStripList(tc.nk, tc.ns, tc.no, tc.nsa)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
