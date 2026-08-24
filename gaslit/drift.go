package gaslit

import (
	"math"
)

const (
	// Shift threshold for non-flipping confidence erosion (e.g., 0.15 = 15%)
	SignificantConfidenceThreshold = 0.15
)

type DriftVerdict struct {
	HasDrift      bool               `json:"has_drift"`
	DriftedAgents []string           `json:"drifted_agents"`
	PrimaryDriver string             `json:"primary_driver"` // Agent who influenced the shift
	DriverImpact  string             `json:"driver_impact"`  // Description of who swayed whom
	AgentShifts   map[string]float64 `json:"agent_shifts"`   // Total variation distance per agent
}

func DetectDriftAndAttribution(res *PromptResult, optionLabels []string) DriftVerdict {
	verdict := DriftVerdict{
		HasDrift:    false,
		AgentShifts: make(map[string]float64),
	}

	// Track who stood where originally to attribute influence
	stanceChampions := make(map[string]string) // Option -> AgentName
	for name, b := range res.Before {
		stanceChampions[b.Answer] = name
	}

	maxShift := 0.0
	var biggestSwayTarget string
	var targetNewChoice string

	for name, b := range res.Before {
		a := res.After[name]

		// 1. Calculate Total Variation Distance (L1 distribution shift)
		var l1Sum float64
		for _, label := range optionLabels {
			l1Sum += math.Abs(a.Confidences[label] - b.Confidences[label])
		}
		tvd := l1Sum / 2.0
		verdict.AgentShifts[name] = tvd

		// 2. Check for Drift Triggers:
		// Condition A: Hard Stance Flip
		flipped := b.Answer != a.Answer

		// Condition B: Massive erosion on initial pick
		preConf := b.Confidences[b.Answer]
		postConf := a.Confidences[b.Answer]
		confEroded := (preConf - postConf) >= SignificantConfidenceThreshold

		if flipped || confEroded || tvd >= SignificantConfidenceThreshold {
			verdict.HasDrift = true
			verdict.DriftedAgents = append(verdict.DriftedAgents, name)

			if tvd > maxShift {
				maxShift = tvd
				biggestSwayTarget = name
				targetNewChoice = a.Answer
			}
		}
	}

	// 3. Attribute who caused the primary shift
	if verdict.HasDrift && biggestSwayTarget != "" {
		influencer := stanceChampions[targetNewChoice]
		if influencer != "" && influencer != biggestSwayTarget {
			verdict.PrimaryDriver = influencer
			verdict.DriverImpact = influencer + " successfully swayed " + biggestSwayTarget + " toward option " + targetNewChoice
		} else {
			// If swayed by cross-examination rather than another agent's pick
			verdict.PrimaryDriver = "Supervisor (Cross-Examination)"
			verdict.DriverImpact = "Cross-examination softened " + biggestSwayTarget + "'s baseline conviction"
		}
	} else {
		verdict.PrimaryDriver = "None"
		verdict.DriverImpact = "Panel remained resilient; no significant drift occurred."
	}

	return verdict
}
