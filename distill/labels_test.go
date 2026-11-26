package distill

import (
	"maps"
	"testing"
)

func TestDatasetLabels(t *testing.T) {
	r := &run{in: Input{Overrides: map[string]Override{
		"_labels": {Labels: map[string]map[string]string{
			"houses": {"staralliance": "Star Alliance"},
			"groups": {"anomaly": "Anomaly"},
		}},
	}}}
	r.label("houses", "staralliance", "StarAlliance")
	r.label("houses", "dis", "Dis")
	r.label("houses", "dis", "DIS")
	got := r.datasetLabels()
	if want := map[string]string{
		"dis":          "Dis",
		"staralliance": "Star Alliance",
	}; !maps.Equal(
		got["houses"],
		want,
	) {
		t.Errorf("houses = %v, want %v", got["houses"], want)
	}
	if got["groups"]["anomaly"] != "Anomaly" {
		t.Errorf("groups = %v", got["groups"])
	}
}
