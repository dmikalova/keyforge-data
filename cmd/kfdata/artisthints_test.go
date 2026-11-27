package main

import (
	"testing"

	"github.com/dmikalova/keyforge-data/artists"
)

func TestDecideHint(t *testing.T) {
	c := func(name string, score float64) artists.Candidate {
		return artists.Candidate{Name: name, Score: score}
	}
	cases := []struct {
		name   string
		cands  []artists.Candidate
		hint   string
		others []string
		want   hintResult
	}{
		{
			"agree",
			[]artists.Candidate{c("Djib", 0.55), c("Rodrigo Camilo", 0.3)},
			"Djib",
			nil,
			hintAgree,
		},
		{"no hint", []artists.Candidate{c("Djib", 0.55)}, "", nil, hintReview},
		{
			"noise, other printings agree",
			[]artists.Candidate{
				c("Regis Torres", 0.39),
			},
			"Caio Monteiro",
			[]string{"Caio Monteiro"},
			hintOnlyHint,
		},
		{
			"noise, new card",
			[]artists.Candidate{c("Jon Bosco", 0.44)},
			"David Otálora",
			nil,
			hintNewCard,
		},
		{
			"other printings have another artist",
			nil, "Djib", []string{"Rodrigo Camilo"}, hintReview,
		},
		{
			"OCR contradicts",
			[]artists.Candidate{c("Oscar Römer", 0.5), c("Jeferson Cordeiro", 0.19)},
			"Jeferson Cordeiro", []string{"Jeferson Cordeiro"}, hintReview,
		},
	}
	for _, tc := range cases {
		readings := len(tc.cands)
		if got := decideHint(tc.cands, readings, tc.hint, tc.others); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
