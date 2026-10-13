package eval

import (
	"math"
	"sort"

	"github.com/ItsArnavSh/gaslit/internal/qa"
)

const (
	eceBins = 10
)

type AgentMetrics struct {
	Agent          string  `json:"agent"`
	Total          int     `json:"total"`
	Answered       int     `json:"answered"`
	Correct        int     `json:"correct"`
	Accuracy       float64 `json:"accuracy"`
	Coverage       float64 `json:"coverage"`
	MeanConfidence float64 `json:"mean_confidence"`
	Brier          float64 `json:"brier"`
	ECE            float64 `json:"ece"`
	MacroF1        float64 `json:"macro_f1,omitempty"`
}

type AggregateMetrics struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

type Bin struct {
	Lo            float64 `json:"lo"`
	Hi            float64 `json:"hi"`
	Count         int     `json:"count"`
	Accuracy      float64 `json:"accuracy"`
	AvgConfidence float64 `json:"avg_confidence"`
}

type DebateAnalysis struct {
	Engaged        int            `json:"engaged"`
	Correct        int            `json:"correct"`
	Accuracy       float64        `json:"accuracy"`
	Quadrants      map[string]int `json:"quadrants,omitempty"`
	AvgDriftAgents float64        `json:"avg_drift_agents,omitempty"`
	DriftRate      float64        `json:"drift_rate,omitempty"`
}

type Analysis struct {
	Mode           string                         `json:"mode"`
	TotalQuestions int                            `json:"total_questions"`
	Agents         []*AgentMetrics                `json:"agents"`
	PerChapter     map[int]*AggregateMetrics      `json:"per_chapter,omitempty"`
	PerDifficulty  map[string]*AggregateMetrics   `json:"per_difficulty,omitempty"`
	Confusion      map[string]map[int]map[int]int `json:"confusion,omitempty"`
	Calibration    map[string][]Bin               `json:"calibration,omitempty"`
	Debate         *DebateAnalysis                `json:"debate,omitempty"`
}

type agentStat struct {
	name        string
	answered    int
	correct     int
	total       int
	sumConf     float64
	chosenConfs []float64
}

func Analyze(res *RunResult) *Analysis {
	if res == nil {
		return nil
	}
	total := len(res.Results)
	an := &Analysis{
		Mode:           string(res.Mode),
		TotalQuestions: total,
		PerChapter:     make(map[int]*AggregateMetrics),
		PerDifficulty:  make(map[string]*AggregateMetrics),
		Confusion:      make(map[string]map[int]map[int]int),
		Calibration:    make(map[string][]Bin),
	}

	agentStats := make(map[string]*agentStat)
	for _, tname := range agentNames(res) {
		agentStats[tname] = &agentStat{
			name:        tname,
			chosenConfs: []float64{},
		}
	}

	debateStats := &DebateAnalysis{Quadrants: make(map[string]int)}
	for i := range res.Results {
		qr := res.Results[i]
		q := qr.Question
		aggChapter(an.PerChapter, q, qr)
		aggDiff(an.PerDifficulty, q, qr)

		if qr.DebateEngaged {
			debateStats.Engaged++
			if qr.DebateCorrect {
				debateStats.Correct++
			}
			if qr.Debate != nil && len(qr.Debate.DriftVerdict.DriftedAgents) > 0 {
				debateStats.AvgDriftAgents += float64(len(qr.Debate.DriftVerdict.DriftedAgents))
				debateStats.DriftRate++
			}
		}

		for _, t := range agentNames(res) {
			st, ok := agentStats[t]
			if !ok {
				st = &agentStat{name: t, chosenConfs: []float64{}}
				agentStats[t] = st
			}
			st.total++
			ans, ok := qr.Answers[t]
			if !ok || ans == nil || ans.Chosen < 0 || ans.Err != "" {
				continue
			}
			st.answered++
			if len(ans.Confidences) > 0 {
				lbl := qa.Labels(len(q.Options))[ans.Chosen]
				if v, ok := ans.Confidences[lbl]; ok && v >= 0 && v <= 1 {
					st.chosenConfs = append(st.chosenConfs, v)
					st.sumConf += v
				}
			}
			if ans.Correct {
				st.correct++
			}
			ensureConf(an.Confusion, t, q.Answer, ans.Chosen)
			if len(ans.Confidences) > 0 {
				lbl := qa.Labels(len(q.Options))[ans.Chosen]
				if v, ok := ans.Confidences[lbl]; ok && v >= 0 && v <= 1 {
					an.recordCal(t, v, ans.Correct)
				}
			}
		}
	}

	if debateStats.Engaged > 0 {
		debateStats.Accuracy = float64(debateStats.Correct) / float64(debateStats.Engaged)
		if debateStats.DriftRate > 0 {
			debateStats.AvgDriftAgents = debateStats.AvgDriftAgents / debateStats.DriftRate
		}
		an.Debate = debateStats
	}

	agents := make([]*AgentMetrics, 0, len(agentStats))
	for _, st := range agentStats {
		m := &AgentMetrics{Agent: st.name, Total: st.total, Answered: st.answered, Correct: st.correct}
		if st.answered > 0 {
			m.Accuracy = float64(st.correct) / float64(st.answered)
			m.Coverage = float64(st.answered) / float64(st.total)
			m.MeanConfidence = st.sumConf / float64(st.answered)
			m.Brier = computeBrier(st, res)
			m.ECE = computeECE(an.Calibration[st.name])
		}
		agents = append(agents, m)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Agent < agents[j].Agent })
	an.Agents = agents
	return an
}

func agentNames(res *RunResult) []string {
	set := make(map[string]bool)
	for i := range res.Results {
		for k := range res.Results[i].Answers {
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func aggChapter(per map[int]*AggregateMetrics, q qa.Question, qr QuestionResult) {
	if per[q.Chapter] == nil {
		per[q.Chapter] = &AggregateMetrics{}
	}
	a := per[q.Chapter]
	a.Total++
	if qr.DebateEngaged {
		if qr.DebateCorrect {
			a.Correct++
		}
		return
	}
	anyCorrect := false
	for _, ans := range qr.Answers {
		if ans != nil && ans.Correct {
			anyCorrect = true
			break
		}
	}
	if anyCorrect {
		a.Correct++
	}
}

func aggDiff(per map[string]*AggregateMetrics, q qa.Question, qr QuestionResult) {
	key := q.Difficulty
	if key == "" {
		key = "mixed"
	}
	if per[key] == nil {
		per[key] = &AggregateMetrics{}
	}
	a := per[key]
	a.Total++
	if qr.DebateEngaged {
		if qr.DebateCorrect {
			a.Correct++
		}
		return
	}
	anyCorrect := false
	for _, ans := range qr.Answers {
		if ans != nil && ans.Correct {
			anyCorrect = true
			break
		}
	}
	if anyCorrect {
		a.Correct++
	}
}

func ensureConf(conf map[string]map[int]map[int]int, agent string, trueIdx, chosenIdx int) {
	if conf[agent] == nil {
		conf[agent] = make(map[int]map[int]int)
	}
	if conf[agent][trueIdx] == nil {
		conf[agent][trueIdx] = make(map[int]int)
	}
	conf[agent][trueIdx][chosenIdx]++
}

func (an *Analysis) recordCal(agent string, conf float64, corr bool) {
	if an.Calibration[agent] == nil {
		an.Calibration[agent] = make([]Bin, eceBins)
		binWidth := 1.0 / float64(eceBins)
		for i := 0; i < eceBins; i++ {
			lo := float64(i) * binWidth
			hi := lo + binWidth
			if i == eceBins-1 {
				hi = 1.0
			}
			an.Calibration[agent][i] = Bin{Lo: lo, Hi: hi}
		}
	}
	bin := int(math.Floor(conf / (1.0 / float64(eceBins))))
	if bin < 0 {
		bin = 0
	}
	if bin >= eceBins {
		bin = eceBins - 1
	}
	b := an.Calibration[agent][bin]
	b.Count++
	b.AvgConfidence += conf
	if corr {
		b.Accuracy += 1.0
	}
	an.Calibration[agent][bin] = b
}

func computeBrier(st *agentStat, res *RunResult) float64 {
	if len(st.chosenConfs) == 0 {
		return 0
	}
	corrSeq := []float64{}
	for i := range res.Results {
		qr := res.Results[i]
		ans, ok := qr.Answers[st.name]
		if !ok || ans == nil || ans.Chosen < 0 || ans.Err != "" {
			continue
		}
		if len(ans.Confidences) > 0 {
			if ans.Correct {
				corrSeq = append(corrSeq, 1.0)
			} else {
				corrSeq = append(corrSeq, 0.0)
			}
		}
	}
	if len(corrSeq) != len(st.chosenConfs) {
		n := len(corrSeq)
		if len(st.chosenConfs) < n {
			n = len(st.chosenConfs)
		}
		if n == 0 {
			return 0
		}
		corrSeq = corrSeq[:n]
		st.chosenConfs = st.chosenConfs[:n]
	}
	var sum float64
	for i := range st.chosenConfs {
		diff := st.chosenConfs[i] - corrSeq[i]
		sum += diff * diff
	}
	return sum / float64(len(st.chosenConfs))
}

func computeECE(bins []Bin) float64 {
	if len(bins) == 0 {
		return 0
	}
	total := 0
	for i := range bins {
		total += bins[i].Count
	}
	if total == 0 {
		return 0
	}
	var ece float64
	for i := range bins {
		if bins[i].Count == 0 {
			continue
		}
		acc := bins[i].Accuracy / float64(bins[i].Count)
		conf := bins[i].AvgConfidence / float64(bins[i].Count)
		ece += float64(bins[i].Count) * math.Abs(acc-conf) / float64(total)
	}
	return ece
}
