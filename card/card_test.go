package card

import (
	"encoding/json/v2"
	"testing"
)

func TestPowerRoundTrips(t *testing.T) {
	for _, p := range []Power{{N: 0}, {N: 7}, {X: true}} {
		b, err := json.Marshal(Card{Power: &p})
		if err != nil {
			t.Fatal(err)
		}
		var c Card
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		if c.Power == nil || *c.Power != p {
			t.Errorf("power %v round-trips to %v (%s)", p, c.Power, b)
		}
	}
	var c Card
	if err := json.Unmarshal([]byte(`{"power":"4"}`), &c); err != nil || c.Power.N != 4 {
		t.Errorf("power 4: %v %v", c.Power, err)
	}
}
