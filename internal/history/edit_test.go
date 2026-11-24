package history

import "testing"

func TestLastEdit(t *testing.T) {
	tail := []byte(`{"type":"user","uuid":"u1","toolUseResult":{"filePath":"/p/a.go","structuredPatch":[{"oldStart":10,"newStart":10,"lines":[" ctx"," ctx2","-old","+new"]}]}}
{"type":"user","uuid":"u2","toolUseResult":{"type":"text","file":{"filePath":"/p/read.go"}}}
{"type":"user","uuid":"u3","toolUseResult":{"type":"create","filePath":"/p/new.go","structuredPatch":[]}}
{"type":"user","uuid":"u4","toolUseResult":{"filePath":"/p/b.go","content":"x"}}
`)
	e, ok := lastEdit(tail)
	if !ok || e.File != "/p/new.go" || e.Line != 1 || e.Key != "u3" {
		t.Fatalf("create: %+v %v", e, ok)
	}
	e, ok = lastEdit(tail[:len(`{"type":"user","uuid":"u1","toolUseResult":{"filePath":"/p/a.go","structuredPatch":[{"oldStart":10,"newStart":10,"lines":[" ctx"," ctx2","-old","+new"]}]}}`)])
	if !ok || e.File != "/p/a.go" || e.Line != 12 {
		t.Fatalf("edit: %+v %v", e, ok)
	}
}
