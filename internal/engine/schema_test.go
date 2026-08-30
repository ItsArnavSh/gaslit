package engine

import (
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

func TestSchemaFor(t *testing.T) {
	g := rawEngine(t)
	if _, ok := g.schemaFor([]string{"a"}).(*core.MCQSchema); !ok {
		t.Error("expected MCQ schema for optioned question")
	}
	if _, ok := g.schemaFor(nil).(*core.OpenEndedSchema); !ok {
		t.Error("expected open-ended schema for option-less question")
	}
}
