package ingest

import "testing"

func TestValidateCoordinates(t *testing.T) {
	cases := []struct {
		lat   float64
		lng   float64
		valid bool
	}{
		{37.0, -122.0, true},
		{90.1, 0, false},
		{-91, 0, false},
		{0, 181, false},
		{0, -181, false},
	}
	for _, c := range cases {
		err := ValidateCoordinates(c.lat, c.lng)
		if c.valid && err != nil {
			t.Fatalf("expected valid for %v", c)
		}
		if !c.valid && err == nil {
			t.Fatalf("expected invalid for %v", c)
		}
	}
}
