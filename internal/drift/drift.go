// Package drift implements the pluggable drift metrics, influence attribution
// strategies, and the pure drift analysis used by the gaslit engine.
package drift

import (
	"math"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// TVDRift is the total variation distance (L1 distance / 2).
func TVDRift(labels []string, before, after *core.Answer) float64 {
	var sum float64
	for _, l := range labels {
		sum += math.Abs(after.Confidences[l] - before.Confidences[l])
	}
	return sum / 2.0
}

// KLDistance is the Kullback-Leibler divergence from before to after, with
// epsilon smoothing to keep it finite.
func KLDistance(labels []string, before, after *core.Answer) float64 {
	const eps = 1e-9
	var sum float64
	for _, l := range labels {
		p := core.Clamp01(before.Confidences[l])
		q := core.Clamp01(after.Confidences[l])
		if p == 0 {
			p = eps
		}
		if q == 0 {
			q = eps
		}
		sum += p * math.Log(p/q)
	}
	return sum
}

// JSDistance is the Jensen-Shannon divergence, a symmetric, bounded variant of
// KL divergence.
func JSDistance(labels []string, before, after *core.Answer) float64 {
	const eps = 1e-9
	var sum float64
	for _, l := range labels {
		p := core.Clamp01(before.Confidences[l])
		q := core.Clamp01(after.Confidences[l])
		m := (p + q) / 2.0
		if p == 0 {
			p = eps
		}
		if q == 0 {
			q = eps
		}
		if m == 0 {
			m = eps
		}
		sum += 0.5 * (p*math.Log(p/m) + q*math.Log(q/m))
	}
	return sum
}

// MaxChange is the maximum absolute per-option confidence change.
func MaxChange(labels []string, before, after *core.Answer) float64 {
	var max float64
	for _, l := range labels {
		if d := math.Abs(after.Confidences[l] - before.Confidences[l]); d > max {
			max = d
		}
	}
	return max
}

// ChampionAttribution credits the agent who originally championed the option
// the biggest sway target moved toward; otherwise it credits the supervisor's
// cross-examination.
func ChampionAttribution(res *core.PromptResult, labels []string, verdict *core.DriftVerdict) {
	stanceChampions := make(map[string]string) // Option -> AgentName
	for name, b := range res.Before {
		stanceChampions[b.Answer] = name
	}

	biggestSwayTarget, targetNewChoice := biggestShiftAgent(res, verdict)

	if biggestSwayTarget == "" {
		verdict.PrimaryDriver = "None"
		verdict.DriverImpact = "Panel remained resilient; no significant drift occurred."
		return
	}

	influencer := stanceChampions[targetNewChoice]
	if influencer != "" && influencer != biggestSwayTarget {
		verdict.PrimaryDriver = influencer
		verdict.DriverImpact = influencer + " successfully swayed " + biggestSwayTarget + " toward option " + targetNewChoice
	} else {
		verdict.PrimaryDriver = "Supervisor (Cross-Examination)"
		verdict.DriverImpact = "Cross-examination softened " + biggestSwayTarget + "'s baseline conviction"
	}
}

// ShiftWeightedAttribution credits the panelist whose own confidence in the
// target's new option rose the most (i.e. who reinforced the target's flip).
func ShiftWeightedAttribution(res *core.PromptResult, labels []string, verdict *core.DriftVerdict) {
	biggestSwayTarget, targetNewChoice := biggestShiftAgent(res, verdict)
	if biggestSwayTarget == "" {
		verdict.PrimaryDriver = "None"
		verdict.DriverImpact = "Panel remained resilient; no significant drift occurred."
		return
	}

	bestInfluencer := ""
	bestGain := 0.0
	for name, b := range res.Before {
		if name == biggestSwayTarget {
			continue
		}
		a := res.After[name]
		pre := b.Confidences[targetNewChoice]
		post := a.Confidences[targetNewChoice]
		if gain := post - pre; gain > bestGain {
			bestGain = gain
			bestInfluencer = name
		}
	}

	if bestInfluencer != "" {
		verdict.PrimaryDriver = bestInfluencer
		verdict.DriverImpact = bestInfluencer + " reinforced option " + targetNewChoice + " while " + biggestSwayTarget + " shifted toward it"
	} else {
		verdict.PrimaryDriver = "Supervisor (Cross-Examination)"
		verdict.DriverImpact = "Cross-examination softened " + biggestSwayTarget + "'s baseline conviction"
	}
}

// CorrelationAttribution credits the panelist whose final position is closest
// to the sway target's new position; otherwise the supervisor.
func CorrelationAttribution(res *core.PromptResult, labels []string, verdict *core.DriftVerdict) {
	biggestSwayTarget, targetNewChoice := biggestShiftAgent(res, verdict)
	if biggestSwayTarget == "" {
		verdict.PrimaryDriver = "None"
		verdict.DriverImpact = "Panel remained resilient; no significant drift occurred."
		return
	}

	bestInfluencer := ""
	bestDist := math.MaxFloat64
	for name, a := range res.After {
		if name == biggestSwayTarget {
			continue
		}
		dist := TVDRift(labels, a, res.After[biggestSwayTarget])
		if dist < bestDist {
			bestDist = dist
			bestInfluencer = name
		}
	}

	if bestInfluencer != "" {
		verdict.PrimaryDriver = bestInfluencer
		verdict.DriverImpact = bestInfluencer + " ended the debate closest to option " + targetNewChoice + " alongside " + biggestSwayTarget
	} else {
		verdict.PrimaryDriver = "Supervisor (Cross-Examination)"
		verdict.DriverImpact = "Cross-examination softened " + biggestSwayTarget + "'s baseline conviction"
	}
}

func biggestShiftAgent(res *core.PromptResult, verdict *core.DriftVerdict) (name, targetNewChoice string) {
	maxShift := -1.0
	for _, drifted := range verdict.DriftedAgents {
		if shift := verdict.AgentShifts[drifted]; shift > maxShift {
			maxShift = shift
			name = drifted
		}
	}
	if name != "" {
		targetNewChoice = res.After[name].Answer
	}
	return name, targetNewChoice
}

// Analyze computes per-agent shifts, flags drifted agents against the
// thresholds, and runs the given attribution function. For open-ended
// questions (no labels / no confidences) it uses lexical answer distance.
func Analyze(res *core.PromptResult, labels []string, thr core.Thresholds, metric core.DriftMetric, attribution core.AttributionFn) core.DriftVerdict {
	verdict := core.DriftVerdict{AgentShifts: make(map[string]float64)}

	for name, b := range res.Before {
		a := res.After[name]
		if a == nil {
			continue
		}

		var shift float64
		if len(labels) == 0 || len(b.Confidences) == 0 {
			shift = lexicalShift(b, a)
		} else {
			shift = metric(labels, b, a)
		}
		verdict.AgentShifts[name] = shift

		flipped := b.Answer != a.Answer
		eroded := false
		if len(b.Confidences) > 0 {
			pre := b.Confidences[b.Answer]
			post := a.Confidences[b.Answer]
			eroded = (pre - post) >= thr.DriftSignificance
		}

		if flipped || eroded || shift >= thr.DriftSignificance {
			verdict.HasDrift = true
			verdict.DriftedAgents = append(verdict.DriftedAgents, name)
		}
	}

	if !verdict.HasDrift {
		verdict.PrimaryDriver = "None"
		verdict.DriverImpact = "Panel remained resilient; no significant drift occurred."
		return verdict
	}

	attribution(res, labels, &verdict)
	return verdict
}

// lexicalShift measures how much two answers differ, as 1 minus the Jaccard
// similarity of their word sets. It is used for open-ended questions.
func lexicalShift(before, after *core.Answer) float64 {
	if before.Answer == after.Answer {
		return 0.0
	}
	u := core.Tokenize(before.Answer)
	v := core.Tokenize(after.Answer)
	if len(u) == 0 && len(v) == 0 {
		return 0.0
	}
	inter, union := 0, 0
	for w := range u {
		if v[w] {
			inter++
		}
	}
	union = len(u)
	for w := range v {
		if !u[w] {
			union++
		}
	}
	if union == 0 {
		return 0.0
	}
	return 1.0 - float64(inter)/float64(union)
}
