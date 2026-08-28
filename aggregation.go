package gaslit

// AggregationFn derives the winning option from the panel's final positions.
// It returns the winning option label, or "" when there is no clear winner.
type AggregationFn func(after map[string]*Answer, labels []string, weights map[string]float64) string

// MajorityVote picks the option chosen by the most panelists.
func MajorityVote(after map[string]*Answer, labels []string, weights map[string]float64) string {
	votes := make(map[string]int)
	for _, a := range after {
		votes[a.Answer]++
	}
	return topOption(votes, len(after))
}

// ConfidenceWeightedVote weights each panelist's vote by their confidence in
// their chosen option.
func ConfidenceWeightedVote(after map[string]*Answer, labels []string, weights map[string]float64) string {
	votes := make(map[string]float64)
	for _, a := range after {
		votes[a.Answer] += a.Confidences[a.Answer]
	}
	return topOptionFloat(votes)
}

// WeightedVote weights each panelist's vote by their configured agent weight
// (default 1.0).
func WeightedVote(after map[string]*Answer, labels []string, weights map[string]float64) string {
	votes := make(map[string]float64)
	for name, a := range after {
		w := weights[name]
		if w == 0 {
			w = 1.0
		}
		votes[a.Answer] += w
	}
	return topOptionFloat(votes)
}

// ConfidenceWeightedWeightedVote combines agent weights with confidence.
func ConfidenceWeightedWeightedVote(after map[string]*Answer, labels []string, weights map[string]float64) string {
	votes := make(map[string]float64)
	for name, a := range after {
		w := weights[name]
		if w == 0 {
			w = 1.0
		}
		votes[a.Answer] += w * a.Confidences[a.Answer]
	}
	return topOptionFloat(votes)
}

func topOption(votes map[string]int, total int) string {
	best := ""
	bestN := 0
	for opt, n := range votes {
		if n > bestN {
			best, bestN = opt, n
		}
	}
	if bestN <= total/2 {
		return ""
	}
	return best
}

func topOptionFloat(votes map[string]float64) string {
	best := ""
	var bestV float64
	for opt, v := range votes {
		if v > bestV {
			best, bestV = opt, v
		}
	}
	if bestV == 0 {
		return ""
	}
	return best
}
