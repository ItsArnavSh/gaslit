package gaslit

import (
	"reflect"
	"testing"
)

func TestGrillTargets(t *testing.T) {
	verdict := DriftVerdict{
		HasDrift:      true,
		DriftedAgents: []string{"econ", "cult", "pol"},
		PrimaryDriver: "econ",
		AgentShifts:   map[string]float64{"econ": 0.9, "cult": 0.4, "pol": 0.2},
	}

	co := CallOptions{GrillCount: 2}

	if got := grillTargets(verdict, CallOptions{GrillStrategy: GrillNone}); got != nil {
		t.Fatalf("GrillNone should return nil, got %v", got)
	}

	if got := grillTargets(verdict, co.with(GrillAllDrifted)); len(got) != 3 {
		t.Fatalf("GrillAllDrifted should return 3, got %v", got)
	}

	if got := grillTargets(verdict, co.with(GrillInfluencer)); !reflect.DeepEqual(got, []string{"econ"}) {
		t.Fatalf("GrillInfluencer should return econ, got %v", got)
	}

	if got := grillTargets(verdict, co.with(GrillSingleSuspect)); !reflect.DeepEqual(got, []string{"econ"}) {
		t.Fatalf("GrillSingleSuspect should return econ, got %v", got)
	}

	top2 := grillTargets(verdict, co.with(GrillTopN))
	if !reflect.DeepEqual(top2, []string{"econ", "cult"}) {
		t.Fatalf("GrillTopN(2) should return [econ cult], got %v", top2)
	}

	// supervisor-driven drift falls back to most drifted agent.
	verdict2 := verdict
	verdict2.PrimaryDriver = "Supervisor (Cross-Examination)"
	if got := grillTargets(verdict2, co.with(GrillInfluencer)); !reflect.DeepEqual(got, []string{"econ"}) {
		t.Fatalf("influencer fallback should return econ, got %v", got)
	}
}

func TestParseConfidence(t *testing.T) {
	if v, err := parseConfidence(`{"confidence":0.8}`); err != nil || v != 0.8 {
		t.Fatalf("got %v, %v", v, err)
	}
	if _, err := parseConfidence(`{"confidence":1.5}`); err == nil {
		t.Fatal("expected range error")
	}
	if _, err := parseConfidence(`no json`); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMean(t *testing.T) {
	if mean(nil) != 0 {
		t.Fatal("mean of nil should be 0")
	}
	if mean([]float64{1, 2, 3}) != 2 {
		t.Fatalf("mean = %v, want 2", mean([]float64{1, 2, 3}))
	}
}

func (co CallOptions) with(s GrillStrategy) CallOptions {
	co.GrillStrategy = s
	return co
}
