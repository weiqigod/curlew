package websocket

import (
	"testing"
)

func TestMessageBuffer_PushAndMatchFIFO(t *testing.T) {
	b := &messageBuffer{}
	b.push([]byte("a"))
	b.push([]byte("b"))
	b.push([]byte("c"))

	idx, frame := b.takeMatch(func(f []byte) bool { return string(f) == "b" })
	if idx != 1 {
		t.Errorf("idx = %d, want 1", idx)
	}
	if string(frame) != "b" {
		t.Errorf("frame = %q, want b", frame)
	}
	if b.len() != 2 {
		t.Errorf("len = %d, want 2 after removing b", b.len())
	}

	// FIFO order preserved: next match-all returns "a" (the oldest remaining).
	_, next := b.takeMatch(func([]byte) bool { return true })
	if string(next) != "a" {
		t.Errorf("next = %q, want a", next)
	}
}

func TestMessageBuffer_TakeMatchNoMatch(t *testing.T) {
	b := &messageBuffer{}
	b.push([]byte("x"))
	b.push([]byte("y"))

	idx, frame := b.takeMatch(func(f []byte) bool { return string(f) == "z" })
	if idx != -1 {
		t.Errorf("idx = %d, want -1 for no match", idx)
	}
	if frame != nil {
		t.Errorf("frame = %v, want nil for no match", frame)
	}
	if b.len() != 2 {
		t.Errorf("len = %d, want 2 (unchanged)", b.len())
	}
}

func TestMessageBuffer_WarningAt101(t *testing.T) {
	b := &messageBuffer{}

	// Push 100 frames — no warning yet.
	for i := 0; i < 100; i++ {
		b.push([]byte("x"))
	}
	if w := b.drainWarning(); w != "" {
		t.Errorf("expected no warning at 100 frames, got %q", w)
	}

	// Push frame 101 — warning should fire.
	b.push([]byte("x"))
	w := b.drainWarning()
	if w == "" {
		t.Error("expected warning after 101 frames, got empty string")
	}

	// drainWarning is destructive — second call returns empty.
	if b.drainWarning() != "" {
		t.Error("warning should be one-shot: second drain should return empty")
	}
}

func TestMessageBuffer_WarningNotRepeated(t *testing.T) {
	b := &messageBuffer{}

	// Push 250 frames.
	for i := 0; i < 250; i++ {
		b.push([]byte("x"))
	}

	// Only one warning across all 250 pushes.
	if b.drainWarning() == "" {
		t.Error("expected exactly one warning")
	}
	if b.drainWarning() != "" {
		t.Error("warning must be one-shot, not repeated")
	}
}

func TestMessageBuffer_CopiesFrameData(t *testing.T) {
	// Mutating the original slice must not affect buffered data.
	b := &messageBuffer{}
	data := []byte("hello")
	b.push(data)
	data[0] = 'X' // mutate original

	_, frame := b.takeMatch(func([]byte) bool { return true })
	if string(frame) != "hello" {
		t.Errorf("frame = %q, want hello (buffer must copy)", frame)
	}
}
