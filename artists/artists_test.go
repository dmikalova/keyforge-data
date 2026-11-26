package artists

import (
	"maps"
	"testing"
)

var known = []string{
	"Caio Monteiro", "Caravan Studio", "Chris Bjors", "Gong Studios",
	"Grigory Serov", "Jorge Ramos", "Michele Giorgi", "Radial Studio",
}

// The readings of the known artists are real artistocr output. The
// readings of the unknown artists are made up from what the cards print.
func TestDecide(t *testing.T) {
	tests := []struct {
		name     string
		readings []string
		want     string
		ok       bool
	}{
		{
			name:     "noisy readings of a known artist",
			readings: []string{"Geigory Seior", "g00 Scrov", "h Geigory Scion", "Geigory Stior"},
			want:     "Grigory Serov",
			ok:       true,
		},
		{
			name: "worn frame",
			readings: []string{
				"Gaio Meeteise",
				"Gaio Meeteiso",
				"Golicin»",
				"Caio Meeteino",
				"Go!",
			},
			want: "Caio Monteiro",
			ok:   true,
		},
		{
			name:     "an artist that is not known",
			readings: []string{"Jorge Ri Marquez", "Jorge R. Marquez", "Jorge R Marguez"},
			ok:       false,
		},
		{
			name:     "a studio that is not known",
			readings: []string{"Limetown Studios", "Limetown Studios", "Lmetown Studios"},
			ok:       false,
		},
		{
			name: "no readings",
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Decide(Match(tt.readings, known, nil, nil))
			if ok != tt.ok || got != tt.want {
				t.Errorf("Decide() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// The readings are real artistocr output: Alaka's Brew (worldscollide 002)
// and past readings of other Michele Giorgi cards.
func TestDecideExamples(t *testing.T) {
	readings := []string{"Mobek Goxgi", "Mxbel Coxgi", "Motel Gorgi", "MKhoe Gogi", "Mokel Coxgi"}
	if name, ok := Decide(Match(readings, known, nil, nil)); ok {
		t.Fatalf("Decide() without examples = %q, want not sure", name)
	}
	ex := Examples{}
	ex.Add("Michele Giorgi", []string{"Mobele Goxgi", "Mxbek Goxgi", "Mibele Gexgi", "Motel Coxgi"})
	ex.Add("Gong Studios", []string{"Gong Stadios", "Gong Seusos", "Cong Sed"})
	got, ok := Decide(Match(readings, known, ex, nil))
	if !ok || got != "Michele Giorgi" {
		t.Errorf("Decide() with examples = %q, %v; want Michele Giorgi, true", got, ok)
	}
}

// Cards print some artists in two ways. The aliases make one candidate of
// the two spellings, so a reading of either spelling is a sure match.
func TestDecideAliases(t *testing.T) {
	names := []string{"Mateusz Mizak", "Gong Studios", "Michele Giorgi"}
	aliases := Aliases{"Matthew Mizak": "Mateusz Mizak"}
	for _, readings := range [][]string{
		{"Matthew Mizak", "Matthew Mizak", "Mathew Mizak"},
		{"Mateusz Mizak", "Mateusz Mizak", "Mateuss Mizak"},
	} {
		got, ok := Decide(Match(readings, names, nil, aliases))
		if !ok || got != "Mateusz Mizak" {
			t.Errorf("Decide(%v) = %q, %v; want Mateusz Mizak, true", readings, got, ok)
		}
	}
	if got := aliases.Canonical("matthew  mizak"); got != "Mateusz Mizak" {
		t.Errorf("Canonical() = %q, want Mateusz Mizak", got)
	}
	m := map[string]map[string]string{"cota": {"101": "Matthew Mizak", "102": "Gong Studios"}}
	aliases.CanonicalAll(m)
	if m["cota"]["101"] != "Mateusz Mizak" || m["cota"]["102"] != "Gong Studios" {
		t.Errorf("CanonicalAll() = %v", m)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	want := map[string]string{
		"001": "Gong Studios",
		"A08": "Caio Monteiro",
		"010": `A "quoted" name`,
	}
	if err := Save(dir, "worldscollide", want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got["worldscollide"], want) {
		t.Errorf("Load() = %v, want %v", got["worldscollide"], want)
	}
}

func TestSaveKnown(t *testing.T) {
	dir := t.TempDir()
	if err := SaveKnown(
		dir,
		[]string{"Ângelo Bortolini", "Zed", "Adam Vehige", "Zed"},
	); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKnown(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Adam Vehige", "Ângelo Bortolini", "Zed"}
	if len(got) != len(want) {
		t.Fatalf("LoadKnown() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("LoadKnown()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
