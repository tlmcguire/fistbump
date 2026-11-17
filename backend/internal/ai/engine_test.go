package ai

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// fake is a provider whose answer the test chooses.
type fake struct {
	name   string
	ready  bool
	drafts []Draft
	err    error
}

func (f fake) Name() string { return f.name }
func (f fake) Ready() bool  { return f.ready }
func (f fake) Suggest(context.Context, Request) ([]Draft, error) {
	return f.drafts, f.err
}

func TestOrder(t *testing.T) {
	local := fake{name: EngineLocal, ready: true}
	idle := fake{name: EngineRemote, ready: false}
	cases := []struct {
		name    string
		engines Engines
		mode    string
		want    []string
	}{
		{"auto uses every ready engine", Engines{Local: local, Remote: idle, Rules: Rules{}}, "auto", []string{"local", "rules"}},
		{"auto with only rules", Engines{Rules: Rules{}}, "auto", []string{"rules"}},
		{"forced engine", Engines{Local: local, Rules: Rules{}}, "local", []string{"local"}},
		{"forced engine that is missing", Engines{Rules: Rules{}}, "local", nil},
		{"forced engine that is not ready", Engines{Remote: idle, Rules: Rules{}}, "remote", nil},
		{"unknown mode", Engines{Rules: Rules{}}, "fast", nil},
	}
	for _, c := range cases {
		if got := c.engines.Order(c.mode); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Order(%q) = %v, want %v", c.name, c.mode, got, c.want)
		}
	}
}

func TestRunForcedEngineUnavailable(t *testing.T) {
	_, _, err := Engines{Rules: Rules{}}.Run(context.Background(), "local", Request{})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestRunAutoFallsThroughOnError(t *testing.T) {
	e := Engines{Local: fake{name: EngineLocal, ready: true, err: errors.New("model crashed")}, Rules: Rules{}}
	req := Request{
		Analysis: matching("sql"),
		Targets:  []Target{{Section: "skills", Original: "excel, sql"}},
	}
	drafts, engine, err := e.Run(context.Background(), "auto", req)
	if err != nil || engine != EngineRules || len(drafts) != 1 || drafts[0].Engine != EngineRules {
		t.Errorf("got %+v, %q, %v", drafts, engine, err)
	}
}

func TestRunForcedEngineReturnsItsError(t *testing.T) {
	boom := errors.New("model crashed")
	e := Engines{Local: fake{name: EngineLocal, ready: true, err: boom}, Rules: Rules{}}
	if _, _, err := e.Run(context.Background(), "local", Request{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the engine's error", err)
	}
}

func TestRunFillsGapsWithRules(t *testing.T) {
	skills := Target{Section: "skills", Original: "excel, sql"}
	summary := Target{Section: "summary", Original: "Analyst."}
	modelDraft := Draft{Section: "summary", Original: "Analyst.", Proposed: "Model summary.", Engine: EngineLocal}
	e := Engines{Local: fake{name: EngineLocal, ready: true, drafts: []Draft{modelDraft}}, Rules: Rules{}}

	drafts, engine, err := e.Run(context.Background(), "auto", Request{
		Analysis: matching("sql"),
		Targets:  []Target{summary, skills},
	})
	if err != nil || engine != EngineLocal || len(drafts) != 2 {
		t.Fatalf("got %+v, %q, %v", drafts, engine, err)
	}
	if drafts[0].Engine != EngineLocal || drafts[1].Section != "skills" || drafts[1].Engine != EngineRules {
		t.Errorf("the model's draft stays local and the skipped skills target gets a rules draft: %+v", drafts)
	}
}

func TestRunLabelsRulesWhenModelWroteNothing(t *testing.T) {
	e := Engines{Local: fake{name: EngineLocal, ready: true}, Rules: Rules{}}
	drafts, engine, err := e.Run(context.Background(), "auto", Request{
		Analysis: matching("sql"),
		Targets:  []Target{{Section: "skills", Original: "excel, sql"}},
	})
	if err != nil || engine != EngineRules || len(drafts) != 1 {
		t.Errorf("got %+v, %q, %v", drafts, engine, err)
	}
}

func TestRunStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := (Engines{Rules: Rules{}}).Run(ctx, "auto", Request{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
