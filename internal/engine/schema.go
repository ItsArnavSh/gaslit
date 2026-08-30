package engine

import "github.com/ItsArnavSh/gaslit/internal/core"

// schemaFor returns the schema to use for a given question. Questions with no
// options always use the open-ended schema.
func (g *Gaslit) schemaFor(options []string) core.AnswerSchema {
	if len(options) == 0 {
		return &core.OpenEndedSchema{}
	}
	return g.cfg.AnswerSchema
}
