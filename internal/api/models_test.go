package api

import "testing"

func TestModelsFromHelpParsesAliases(t *testing.T) {
	help := `Options:
  --model <model>                       Model for the current session. Provide
                                        an alias for the latest model (e.g.
                                        'fable', 'opus', or 'sonnet') or a
                                        model's full name (e.g.
                                        'claude-fable-5').
  --no-something                        other flag`
	got := modelsFromHelp(help)
	if len(got) != 3 {
		t.Fatalf("expected 3 aliases, got %d: %+v", len(got), got)
	}
	// Ordered by preference: opus, sonnet, then fable; claude-fable-5 excluded.
	want := []string{"opus", "sonnet", "fable"}
	for i, w := range want {
		if got[i].ID != w {
			t.Fatalf("model %d = %q, want %q", i, got[i].ID, w)
		}
		if got[i].Label == "" {
			t.Fatalf("model %q has empty label", got[i].ID)
		}
	}
}

func TestModelsFromHelpNoModelFlag(t *testing.T) {
	if got := modelsFromHelp("no model flag here"); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestBinaryModelListerFallsBack(t *testing.T) {
	list := BinaryModelLister("this-binary-does-not-exist-xyz")()
	if len(list) == 0 {
		t.Fatal("expected fallback models when the binary is missing")
	}
}
