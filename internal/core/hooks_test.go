package core

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestMergeHooks(t *testing.T) {
	var baseCalls, overrideCalls atomic.Int64
	base := Hooks{OnBaseline: func(context.Context, string, *Answer) { baseCalls.Add(1) }}
	override := Hooks{OnBaseline: func(context.Context, string, *Answer) { overrideCalls.Add(1) }}

	merged := MergeHooks(base, override)
	merged.OnBaseline(context.Background(), "a", nil)

	if overrideCalls.Load() != 1 {
		t.Error("override hook should be called")
	}
	if baseCalls.Load() != 0 {
		t.Error("base hook should be shadowed by override")
	}
}

func TestMergeHooksPartial(t *testing.T) {
	var driftCalls atomic.Int64
	base := Hooks{OnDrift: func(context.Context, *DriftVerdict) { driftCalls.Add(1) }}
	override := Hooks{OnBaseline: func(context.Context, string, *Answer) {}}

	merged := MergeHooks(base, override)
	merged.OnDrift(context.Background(), nil)

	if driftCalls.Load() != 1 {
		t.Error("base OnDrift should remain when override does not set it")
	}
}
