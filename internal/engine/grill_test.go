package engine

import (
	"reflect"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

func TestGrillTargets(t *testing.T) {
	verdict := core.DriftVerdict{
		HasDrift:      true,
		DriftedAgents: []string{"econ", "cult", "pol"},
		PrimaryDriver: "econ",
		AgentShifts:   map[string]float64{"econ": 0.9, "cult": 0.4, "pol": 0.2},
	}

	co := core.CallOptions{GrillCount: 2}

	if got := grillTargets(verdict, core.CallOptions{GrillStrategy: core.GrillNone}); got != nil {
		t.Fatalf("GrillNone should return nil, got %v", got)
	}

	co.GrillStrategy = core.GrillAllDrifted
	if got := grillTargets(verdict, co); len(got) != 3 {
		t.Fatalf("GrillAllDrifted should return 3, got %v", got)
	}

	co.GrillStrategy = core.GrillInfluencer
	if got := grillTargets(verdict, co); !reflect.DeepEqual(got, []string{"econ"}) {
		t.Fatalf("GrillInfluencer should return econ, got %v", got)
	}

	co.GrillStrategy = core.GrillSingleSuspect
	if got := grillTargets(verdict, co); !reflect.DeepEqual(got, []string{"econ"}) {
		t.Fatalf("GrillSingleSuspect should return econ, got %v", got)
	}

	co.GrillStrategy = core.GrillTopN
	top2 := grillTargets(verdict, co)
	if !reflect.DeepEqual(top2, []string{"econ", "cult"}) {
		t.Fatalf("GrillTopN(2) should return [econ cult], got %v", top2)
	}

	// supervisor-driven drift falls back to most drifted agent.
	verdict2 := verdict
	verdict2.PrimaryDriver = "Supervisor (Cross-Examination)"
	co.GrillStrategy = core.GrillInfluencer
	if got := grillTargets(verdict2, co); !reflect.DeepEqual(got, []string{"econ"}) {
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
