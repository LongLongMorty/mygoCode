package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"mygocode/internal/conversation"
	"mygocode/internal/llm"
	"mygocode/internal/tools"
)

// recordingClient wraps a mockClient and captures the text of every
// conversation it streams, so tests can assert which client handled which
// call (compaction summary vs main turn).
type recordingClient struct {
	inner *mockClient
	mu    sync.Mutex
	texts []string // first user message text per Stream call
}

func newRecordingClient(inner *mockClient) *recordingClient {
	return &recordingClient{inner: inner}
}

func (r *recordingClient) SetSystemPrompt(string) {}

func (r *recordingClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan llm.StreamEvent, <-chan error) {
	msgs := conv.GetMessages()
	var first string
	for _, m := range msgs {
		if m.Role == "user" && m.Content != "" {
			first = m.Content
			break
		}
	}
	r.mu.Lock()
	r.texts = append(r.texts, first)
	r.mu.Unlock()
	return r.inner.Stream(ctx, conv, toolSchemas)
}

func (r *recordingClient) userTexts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.texts))
	copy(out, r.texts)
	return out
}

const summaryResponse = "<summary>compacted summary</summary>"

// compactSetup builds an agent whose context window is small enough that
// auto-compaction fires on the first iteration. mainClient serves the
// main conversation; auxClient (may be nil) is wired as AuxClient.
//
// The conversation needs enough messages that the "keep recent tail" budget
// (minKeepMessages = 5) leaves a prefix to summarize — a single message would
// be kept verbatim and compaction would no-op.
func compactSetup(main *recordingClient, aux *recordingClient) (*Agent, *conversation.Manager) {
	ag := New(main, tools.NewRegistry(), "anthropic")
	ag.AuxClient = aux
	ag.ContextWindow = 5000 // effective window < 0 → always triggers compaction
	ag.MaxIterations = 5
	conv := conversation.NewManager()
	for i := 0; i < 6; i++ {
		conv.AddUserMessage("message number " + string(rune('0'+i)))
	}
	return ag, conv
}

func runQuiet(ag *Agent, conv *conversation.Manager) {
	ch := ag.Run(context.Background(), conv)
	for range ch {
	}
}

// TestAgentAutoCompactUsesAuxClient verifies that when AuxClient is wired,
// the compaction summary request goes to the auxiliary client and the main
// conversation still streams through the main client.
func TestAgentAutoCompactUsesAuxClient(t *testing.T) {
	main := newRecordingClient(&mockClient{responses: [][]llm.StreamEvent{{
		llm.TextDelta{Text: "main turn"},
		llm.StreamEnd{StopReason: "end_turn"},
	}}})
	aux := newRecordingClient(&mockClient{responses: [][]llm.StreamEvent{{
		llm.TextDelta{Text: summaryResponse},
		llm.StreamEnd{StopReason: "end_turn"},
	}}})

	ag, conv := compactSetup(main, aux)
	runQuiet(ag, conv)

	auxTexts := aux.userTexts()
	if len(auxTexts) != 1 || !strings.Contains(auxTexts[0], "create a detailed summary") {
		t.Fatalf("aux client did not receive the compaction summary request: %q", auxTexts)
	}
	mainTexts := main.userTexts()
	if len(mainTexts) != 1 {
		t.Fatalf("main client calls = %d, want 1 (the main turn only)", len(mainTexts))
	}
	if strings.Contains(mainTexts[0], "create a detailed summary") {
		t.Errorf("main client received the summary request instead of the aux client: %q", mainTexts[0])
	}
}

// TestAgentAutoCompactFallsBackToMainClient verifies that without a usable
// AuxClient the compaction summary is handled by the main client (backward
// compatible). It passes a typed-nil *recordingClient to prove the nil guard
// catches the interface-wrapping footgun, not just a plain nil interface.
func TestAgentAutoCompactFallsBackToMainClient(t *testing.T) {
	main := newRecordingClient(&mockClient{responses: [][]llm.StreamEvent{
		{ // call 1: compaction summary
			llm.TextDelta{Text: summaryResponse},
			llm.StreamEnd{StopReason: "end_turn"},
		},
		{ // call 2: main turn after compaction
			llm.TextDelta{Text: "main turn"},
			llm.StreamEnd{StopReason: "end_turn"},
		},
	}})

	ag, conv := compactSetup(main, (*recordingClient)(nil))
	runQuiet(ag, conv)

	texts := main.userTexts()
	if len(texts) < 2 {
		t.Fatalf("main client calls = %d, want >= 2 (summary + main turn)", len(texts))
	}
	if !strings.Contains(texts[0], "create a detailed summary") {
		t.Errorf("first main-client call should be the summary request, got: %q", texts[0])
	}
	if strings.Contains(texts[1], "create a detailed summary") {
		t.Errorf("second main-client call should be the main turn, got: %q", texts[1])
	}
}
