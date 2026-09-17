package progresswatch

import "testing"

func TestDetectorRequiresTwoConsecutiveUnchangedChecks(t *testing.T) {
	detector := NewDetector(100)

	if got := detector.Observe(100, 110); got != Waiting {
		t.Fatalf("first unchanged observation = %v, want Waiting", got)
	}
	if got := detector.Observe(100, 110); got != Stalled {
		t.Fatalf("second unchanged observation = %v, want Stalled", got)
	}
	if got := detector.Observe(100, 110); got != Waiting {
		t.Fatalf("observation after stalled recovery = %v, want Waiting", got)
	}
}

func TestDetectorProgressResetsUnchangedChecks(t *testing.T) {
	detector := NewDetector(100)

	if got := detector.Observe(100, 110); got != Waiting {
		t.Fatalf("first unchanged observation = %v, want Waiting", got)
	}
	if got := detector.Observe(101, 110); got != Progressed {
		t.Fatalf("advanced observation = %v, want Progressed", got)
	}
	if got := detector.Observe(101, 110); got != Waiting {
		t.Fatalf("first unchanged observation after progress = %v, want Waiting", got)
	}
	if got := detector.Observe(101, 110); got != Stalled {
		t.Fatalf("second unchanged observation after progress = %v, want Stalled", got)
	}
}

func TestDetectorAtLatestResetsUnchangedChecks(t *testing.T) {
	detector := NewDetector(100)

	if got := detector.Observe(100, 110); got != Waiting {
		t.Fatalf("first unchanged observation = %v, want Waiting", got)
	}
	if got := detector.Observe(100, 100); got != AtLatest {
		t.Fatalf("caught-up observation = %v, want AtLatest", got)
	}
	if got := detector.Observe(100, 110); got != Waiting {
		t.Fatalf("first unchanged observation after falling behind = %v, want Waiting", got)
	}
}
