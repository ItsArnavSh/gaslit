package eval

import (
		"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	)

const (
	// ISO8601 layout
	timeLayout = "2006-01-02T15:04:05Z"
)

// Markdown renders the Analysis as GitHub-flavored Markdown.
func (an *Analysis) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Datatest Analysis\n\n")
	fmt.Fprintf(&b, "- Mode: **%s**\n", an.Mode)
	fmt.Fprintf(&b, "- Total questions: **%d**\n\n", an.TotalQuestions)

	if len(an.Agents) > 0 {
		b.WriteString("## Agent metrics\n\n")
		b.WriteString("| Agent | Total | Answered | Coverage | Correct | Accuracy | Mean Conf. | Brier | ECE |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
		for _, m := range an.Agents {
			b.WriteString("| ")
			b.WriteString(esc(m.Agent))
			fmt.Fprintf(&b, " | %d | %d | %.3f | %d | %.3f | %.3f | %.3f | %.3f |\n",
				m.Total, m.Answered, m.Coverage, m.Correct, m.Accuracy, m.MeanConfidence, m.Brier, m.ECE)
		}
		b.WriteString("\n")
	}

	if an.Debate != nil {
		b.WriteString("## Debate analysis\n\n")
		fmt.Fprintf(&b, "- Engaged: **%d**\n", an.Debate.Engaged)
		fmt.Fprintf(&b, "- Correct (winner matched ground truth): **%d** (%.3f)\n\n",
			an.Debate.Correct, an.Debate.Accuracy)
		if len(an.Debate.Quadrants) > 0 {
			b.WriteString("### Quadrants\n\n")
			keys := make([]string, 0, len(an.Debate.Quadrants))
			for k := range an.Debate.Quadrants {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteString("| Quadrant | Count |\n|---|---|\n")
			for _, k := range keys {
				fmt.Fprintf(&b, "| %s | %d |\n", k, an.Debate.Quadrants[k])
			}
			b.WriteString("\n")
		}
	}

	if len(an.PerDifficulty) > 0 {
		b.WriteString("## Accuracy by difficulty\n\n")
		b.WriteString("| Difficulty | Total | Correct | Accuracy |\n|---|---|---|---|\n")
		keys := make([]string, 0, len(an.PerDifficulty))
		for k := range an.PerDifficulty {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			a := an.PerDifficulty[k]
			fmt.Fprintf(&b, "| %s | %d | %d | %.3f |\n", k, a.Total, a.Correct, a.Accuracy)
		}
		b.WriteString("\n")
	}

	if len(an.PerChapter) > 0 {
		b.WriteString("## Accuracy by chapter\n\n")
		b.WriteString("| Chapter | Total | Correct | Accuracy |\n|---|---|---|---|\n")
		keys := make([]int, 0, len(an.PerChapter))
		for k := range an.PerChapter {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			a := an.PerChapter[k]
			fmt.Fprintf(&b, "| %d | %d | %d | %.3f |\n", k, a.Total, a.Correct, a.Accuracy)
		}
		b.WriteString("\n")
	}

	if len(an.Confusion) > 0 {
		b.WriteString("## Confusion matrices (chosen → correct true? shown as true->chosen counts)\n\n")
		for agent, mat := range an.Confusion {
			b.WriteString("### " + esc(agent) + "\n\n")
			b.WriteString("Rows = true option index; Columns = chosen option index (counts). Truncated to observed dimensions.\n\n")
			// collect dims
			trues := make([]int, 0, len(mat))
			for t := range mat {
				trues = append(trues, t)
			}
			sort.Ints(trues)
			chosensSet := make(map[int]bool)
			for _, m2 := range mat {
				for c := range m2 {
					chosensSet[c] = true
				}
			}
			chos := make([]int, 0, len(chosensSet))
			for c := range chosensSet {
				chos = append(chos, c)
			}
			sort.Ints(chos)
			if len(chos) == 0 || len(trues) == 0 {
				continue
			}
			b.WriteString("| true\\chosen |")
			for _, c := range chos {
				fmt.Fprintf(&b, " %d |", c)
			}
			b.WriteString("\n|---")
			for range chos {
				b.WriteString("|---")
			}
			b.WriteString("|\n")
			for _, t := range trues {
				fmt.Fprintf(&b, "| %d |", t)
				for _, c := range chos {
					cnt := 0
					if mat[t] != nil {
						cnt = mat[t][c]
					}
					fmt.Fprintf(&b, " %d |", cnt)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	if len(an.Calibration) > 0 {
		b.WriteString("## Calibration (ECE bins)\n\n")
		for agent, bins := range an.Calibration {
			b.WriteString("### " + esc(agent) + "\n\n")
			b.WriteString("| Bin | Count | Accuracy | Avg. Conf. |\n|---|---|---|---|\n")
			for i := range bins {
				if bins[i].Count == 0 {
					continue
				}
				acc := bins[i].Accuracy / float64(bins[i].Count)
				conf := bins[i].AvgConfidence / float64(bins[i].Count)
				fmt.Fprintf(&b, "| [%.2f, %.2f) | %d | %.3f | %.3f |\n",
					bins[i].Lo, bins[i].Hi, bins[i].Count, acc, conf)
			}
			b.WriteString("\n")
		}
	}

	if res := an.ResultsSummary(); res != "" {
		b.WriteString(res)
	}

	return b.String()
}

func (an *Analysis) ResultsSummary() string {
	return ""
}

// JSON renders the Analysis as pretty-printed JSON.
func (an *Analysis) JSON() string {
	data, err := json.MarshalIndent(an, "", "  ")
	if err != nil {
		return "{\"error\":\"" + err.Error() + "\"}"
	}
	return string(data)
}

// Save writes the Markdown report to path.
func (an *Analysis) Save(path string) error {
	return os.WriteFile(path, []byte(an.Markdown()), 0o644)
}

// SaveJSON writes the JSON analysis to path.
func (an *Analysis) SaveJSON(path string) error {
	return os.WriteFile(path, []byte(an.JSON()), 0o644)
}

// Report writes both Markdown and JSON for the run result.
func Report(res *RunResult, mdPath, jsonPath string) (*Analysis, error) {
	an := Analyze(res)
	if mdPath != "" {
		if err := an.Save(mdPath); err != nil {
			return an, err
		}
	}
	if jsonPath != "" {
		if err := an.SaveJSON(jsonPath); err != nil {
			return an, err
		}
	}
	return an, nil
}

// QuestionResult helpers for detailed reporting (optional)
type QuestionResultDetail struct {
	ID             string
	Source         string
	Chapter        int
	ChapterTitle   string
	Question       string
	Options        []string
	Answer         int
	AnswerLabel    string
	AgentAnswers   map[string]*AgentAnswer
	DebateWinner   int
	DebateCorrect  bool
	DebateEngaged  bool
}

func (res *RunResult) Details() []QuestionResultDetail {
	out := make([]QuestionResultDetail, 0, len(res.Results))
	for _, qr := range res.Results {
		q := qr.Question
		d := QuestionResultDetail{
			ID:            q.ID,
			Source:        q.Source,
			Chapter:       q.Chapter,
			ChapterTitle:  q.ChapterTitle,
			Question:      q.Question,
			Options:       append([]string(nil), q.Options...),
			Answer:        q.Answer,
			AnswerLabel:   q.AnswerLabel(),
			AgentAnswers:  qr.Answers,
			DebateWinner:  qr.DebateWinner,
			DebateCorrect: qr.DebateCorrect,
			DebateEngaged: qr.DebateEngaged,
		}
		out = append(out, d)
	}
	return out
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func itoa(i int) string { return strconv.Itoa(i) }
