package gaslit

import (
	"testing"
)

func TestDetectDriftAndAttribution(t *testing.T) {
	labels := []string{"A", "B", "C"}

	base := func(a string, confA, confB, confC float64, reasoning string) *mcqAnswer {
		return &mcqAnswer{
			Answer:    a,
			Reasoning: reasoning,
			Confidences: map[string]float64{
				"A": confA,
				"B": confB,
				"C": confC,
			},
		}
	}

	t.Run("no drift", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*mcqAnswer{"econ": base("A", 0.9, 0.05, 0.05, "r")},
			After:  map[string]*mcqAnswer{"econ": base("A", 0.88, 0.07, 0.05, "r")},
		}
		v := detectDriftAndAttribution(res, labels)
		if v.HasDrift {
			t.Fatalf("expected no drift, got %+v", v)
		}
		if v.PrimaryDriver != "None" {
			t.Fatalf("expected PrimaryDriver None, got %q", v.PrimaryDriver)
		}
	})

	t.Run("stance flip detected", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*mcqAnswer{
				"pol":  base("A", 0.9, 0.05, 0.05, "r"),
				"econ": base("B", 0.05, 0.9, 0.05, "r"),
			},
			After: map[string]*mcqAnswer{
				"pol":  base("B", 0.05, 0.9, 0.05, "r"),
				"econ": base("B", 0.05, 0.9, 0.05, "r"),
			},
		}
		v := detectDriftAndAttribution(res, labels)
		if !v.HasDrift {
			t.Fatal("expected drift on stance flip")
		}
		if len(v.DriftedAgents) != 1 || v.DriftedAgents[0] != "pol" {
			t.Fatalf("unexpected drifted agents: %v", v.DriftedAgents)
		}
		// pol flipped toward B; econ championed B originally and is the driver.
		if v.PrimaryDriver != "econ" {
			t.Fatalf("expected econ as primary driver, got %q", v.PrimaryDriver)
		}
		if v.DriverImpact == "" {
			t.Fatal("expected non-empty driver impact")
		}
	})

	t.Run("confidence erosion without flip", func(t *testing.T) {
		before := base("A", 0.9, 0.05, 0.05, "r")
		after := base("A", 0.5, 0.4, 0.1, "r")
		res := &PromptResult{
			Before: map[string]*mcqAnswer{"cult": before},
			After:  map[string]*mcqAnswer{"cult": after},
		}
		v := detectDriftAndAttribution(res, labels)
		if !v.HasDrift {
			t.Fatal("expected drift on confidence erosion")
		}
		// erosion alone, no flip -> supervisor credited
		if v.PrimaryDriver != "Supervisor (Cross-Examination)" {
			t.Fatalf("expected supervisor attribution, got %q", v.PrimaryDriver)
		}
	})

	t.Run("total variation distance recorded", func(t *testing.T) {
		before := base("A", 1.0, 0.0, 0.0, "r")
		after := base("C", 0.0, 0.0, 1.0, "r")
		res := &PromptResult{
			Before: map[string]*mcqAnswer{"econ": before},
			After:  map[string]*mcqAnswer{"econ": after},
		}
		v := detectDriftAndAttribution(res, labels)
		if got := v.AgentShifts["econ"]; got != 1.0 {
			t.Fatalf("expected TVD 1.0, got %v", got)
		}
	})

	t.Run("swayed by champion attribution", func(t *testing.T) {
		res := &PromptResult{
			Before: map[string]*mcqAnswer{
				"econ": base("A", 0.9, 0.05, 0.05, "r"),
				"pol":  base("B", 0.05, 0.9, 0.05, "r"),
			},
			After: map[string]*mcqAnswer{
				"econ": base("B", 0.05, 0.9, 0.05, "r"),
				"pol":  base("B", 0.05, 0.9, 0.05, "r"),
			},
		}
		v := detectDriftAndAttribution(res, labels)
		if !v.HasDrift {
			t.Fatal("expected drift")
		}
		if v.PrimaryDriver != "pol" {
			t.Fatalf("expected pol as driver, got %q", v.PrimaryDriver)
		}
	})
}
