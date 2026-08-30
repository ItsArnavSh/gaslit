package core

import "testing"

func TestClassifyWith(t *testing.T) {
	thr := Thresholds{DriftSignificance: 0.15, Confidence: 0.55, Consistency: 0.60}
	tests := []struct {
		name    string
		conf    float64
		consist float64
		want    QuadrantState
	}{
		{"peak", 0.9, 0.9, QuadrantPeak},
		{"hallucinating", 0.9, 0.3, QuadrantHallucinating},
		{"kb deficit", 0.3, 0.9, QuadrantKBDeficit},
		{"wrong", 0.3, 0.3, QuadrantWrong},
		{"boundary confidence high", 0.55, 0.61, QuadrantPeak},
		{"boundary confidence low", 0.54, 0.61, QuadrantKBDeficit},
		{"boundary consistency high", 0.61, 0.60, QuadrantPeak},
		{"boundary consistency low", 0.61, 0.59, QuadrantHallucinating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quadrant, rec := ClassifyWith(tt.conf, tt.consist, thr)
			if quadrant != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, quadrant)
			}
			if rec == "" {
				t.Fatal("expected non-empty recommendation")
			}
		})
	}
}

func TestThresholdClassifier(t *testing.T) {
	// ThresholdClassifier uses built-in defaults.
	q, _ := ThresholdClassifier(0.9, 0.9)
	if q != QuadrantPeak {
		t.Fatalf("expected PEAK, got %s", q)
	}
}

func TestReclassifySummary(t *testing.T) {
	s := &FullDebateSummary{
		Diagnostics: map[string]*DiagnosticResult{
			"econ": {AgentName: "econ", ConfidenceScore: 0.9, ConsistencyScore: 0.9, Quadrant: QuadrantPeak},
			"pol":  {AgentName: "pol", ConfidenceScore: 0.9, ConsistencyScore: 0.2, Quadrant: QuadrantPeak},
		},
	}
	thr := Thresholds{Confidence: 0.55, Consistency: 0.60}

	out := ReclassifySummary(s, thr)
	if out.Diagnostics["econ"].Quadrant != QuadrantPeak {
		t.Errorf("econ should stay PEAK, got %s", out.Diagnostics["econ"].Quadrant)
	}
	if out.Diagnostics["pol"].Quadrant != QuadrantHallucinating {
		t.Errorf("pol should be HALLUCINATING, got %s", out.Diagnostics["pol"].Quadrant)
	}

	if ReclassifySummary(nil, thr) != nil {
		t.Error("nil summary should return nil")
	}
}
