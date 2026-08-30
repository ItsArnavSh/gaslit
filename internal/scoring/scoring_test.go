package scoring

import (
	"context"
	"testing"
)

func TestLexicalScorer(t *testing.T) {
	s := &LexicalScorer{}

	score, err := s.Score(context.Background(), "q", "The economy collapsed due to inflation", "The economy collapsed due to inflation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0.9 {
		t.Errorf("identical arguments should score high, got %v", score)
	}

	score, err = s.Score(context.Background(), "q", "Rome fell to economic decay", "Giant space lobsters invaded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score > 0.3 {
		t.Errorf("unrelated arguments should score low, got %v", score)
	}

	score, err = s.Score(context.Background(), "q", "", "")
	if err != nil || score != 0.5 {
		t.Errorf("empty arguments should score 0.5, got %v err %v", score, err)
	}
}
