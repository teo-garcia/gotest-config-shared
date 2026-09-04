package consumer

import "testing"

func TestCovered(t *testing.T) {
	t.Parallel()

	if got := Covered(); got != 1 {
		t.Fatalf("Covered() = %d, want 1", got)
	}
}
