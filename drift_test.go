package gaslit

import (
	"math"
	"testing"
)

func base(a string, confA, confB, confC float64, reasoning string) *Answer {
	return &Answer{
		Answer:    a,
		Reasoning: reasoning,
		Confidences: map[string]float64{
			"A": confA,
			"B": confB,
			"C": confC,
		},
	}
}

func TestDriftMetrics(t *testing.T) {
	labels := []string{"A", "B", "C"}
	before := base("A", 1.0, 0.0, 0.0, "r")
	after := base("C", 0.0, 0.0, 1.0, "r")

	if got := TVDRift(labels, before, after); got != 1.0 {
		t.Errorf("TVD = %v, want 1.0", got)
	}
	if got := MaxChange(labels, before, after); got != 1.0 {
		t.Errorf("MaxChange = %v, want 1.0", got)
	}

	kl := KLDistance(labels, before, after)
	if math.IsInf(kl, 0) || math.IsNaN(kl) || kl < 0 {
		t.Errorf("KL = %v, want finite non-negative", kl)
	}
	js := JSDistance(labels, before, after)
	if math.IsInf(js, 0) || math.IsNaN(js) || js < 0 {
		t.Errorf("JS = %v, want finite non-negative", js)
	}

	// identical distributions -> zero for all metrics
	if TVDRift(labels, before, before) != 0.0 || MaxChange(labels, before, before) != 0.0 {
		t.Error("identical distributions should yield zero shift")
	}
}

func TestLexicalShift(t *testing.T) {
	tests := []struct {
		name    string
		b, a    string
		shifted bool
	}{
		{"same answer", "The economy collapsed", "The economy collapsed", false},
		{"completely different", "The economy collapsed", "The military overextended its frontiers", true},
		{"partial overlap", "Economic inflation destroyed Rome", "Economic factors were decisive", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := lexicalShift(&Answer{Answer: tt.b}, &Answer{Answer: tt.a})
			if tt.shifted && s < 0.15 {
				t.Errorf("shift = %v, want >= 0.15", s)
			}
			if !tt.shifted && s != 0 {
				t.Errorf("shift = %v, want 0", s)
			}
		})
	}
}

func TestAnalyzeDrift(t *testing.T) {
	labels := []string{"A", "B", "C"}
	thr := Thresholds{}.resolve()

	t.Run("no drift", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*Answer{"econ": base("A", 0.9, 0.05, 0.05, "r")},
			After:  map[string]*Answer{"econ": base("A", 0.88, 0.07, 0.05, "r")},
		}
		v := analyzeDrift(res, labels, thr, TVDRift, ChampionAttribution)
		if v.HasDrift {
			t.Fatalf("expected no drift, got %+v", v)
		}
		if v.PrimaryDriver != "None" {
			t.Fatalf("expected PrimaryDriver None, got %q", v.PrimaryDriver)
		}
	})

	t.Run("stance flip champion attribution", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*Answer{
				"pol":  base("A", 0.9, 0.05, 0.05, "r"),
				"econ": base("B", 0.05, 0.9, 0.05, "r"),
			},
			After: map[string]*Answer{
				"pol":  base("B", 0.05, 0.9, 0.05, "r"),
				"econ": base("B", 0.05, 0.9, 0.05, "r"),
			},
		}
		v := analyzeDrift(res, labels, thr, TVDRift, ChampionAttribution)
		if !v.HasDrift {
			t.Fatal("expected drift on stance flip")
		}
		if v.PrimaryDriver != "econ" {
			t.Fatalf("expected econ as driver, got %q", v.PrimaryDriver)
		}
	})

	t.Run("confidence erosion without flip credits supervisor", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*Answer{"cult": base("A", 0.9, 0.05, 0.05, "r")},
			After:  map[string]*Answer{"cult": base("A", 0.5, 0.4, 0.1, "r")},
		}
		v := analyzeDrift(res, labels, thr, TVDRift, ChampionAttribution)
		if !v.HasDrift {
			t.Fatal("expected drift on confidence erosion")
		}
		if v.PrimaryDriver != "Supervisor (Cross-Examination)" {
			t.Fatalf("expected supervisor attribution, got %q", v.PrimaryDriver)
		}
	})

	t.Run("shift weighted attribution", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*Answer{
				"pol":  base("A", 0.9, 0.05, 0.05, "r"),
				"econ": base("B", 0.05, 0.9, 0.05, "r"),
			},
			After: map[string]*Answer{
				"pol":  base("B", 0.05, 0.9, 0.05, "r"),
				"econ": base("B", 0.05, 0.95, 0.0, "r"),
			},
		}
		v := analyzeDrift(res, labels, thr, TVDRift, ShiftWeightedAttribution)
		if !v.HasDrift || v.PrimaryDriver != "econ" {
			t.Fatalf("unexpected verdict: %+v", v)
		}
	})

	t.Run("total variation distance recorded", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*Answer{"econ": base("A", 1.0, 0.0, 0.0, "r")},
			After:  map[string]*Answer{"econ": base("C", 0.0, 0.0, 1.0, "r")},
		}
		v := analyzeDrift(res, labels, thr, TVDRift, ChampionAttribution)
		if got := v.AgentShifts["econ"]; got != 1.0 {
			t.Fatalf("expected TVD 1.0, got %v", got)
		}
	})

	t.Run("open ended uses lexical shift", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*Answer{"econ": {Answer: "Economic collapse drove the fall"}},
			After:  map[string]*Answer{"econ": {Answer: "Barbarian invasions were decisive"}},
		}
		v := analyzeDrift(res, nil, thr, TVDRift, ChampionAttribution)
		if !v.HasDrift {
			t.Fatal("expected lexical drift on open-ended answers")
		}
		if v.AgentShifts["econ"] <= 0 {
			t.Fatalf("expected positive lexical shift, got %v", v.AgentShifts["econ"])
		}
	})
}
