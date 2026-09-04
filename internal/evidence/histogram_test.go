package evidence_test

import (
	"math"
	"testing"

	"github.com/wahidyankf/hippo/internal/evidence"
)

func TestHistogramRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	invalid := evidence.NewHistogram(math.NaN(), 1)
	if invalid.Add(1) {
		t.Fatal("histogram with invalid bounds accepted a value")
	}

	var missing *evidence.Histogram
	if missing.Add(1) {
		t.Fatal("nil histogram accepted a value")
	}
	if _, ok := missing.Quantile(.5); ok {
		t.Fatal("nil histogram returned a quantile")
	}

	histogram := evidence.NewHistogram(10, 1)
	for _, value := range []float64{-1, 11, math.NaN(), math.Inf(1)} {
		if histogram.Add(value) {
			t.Fatalf("histogram accepted invalid value %v", value)
		}
	}
	for _, proportion := range []float64{0, -1, 1.1, math.NaN(), math.Inf(1)} {
		if _, ok := histogram.Quantile(proportion); ok {
			t.Fatalf("histogram accepted invalid quantile %v", proportion)
		}
	}
}

func TestHistogramReturnsConservativeNearestRank(t *testing.T) {
	t.Parallel()

	histogram := evidence.NewHistogram(10, .5)
	for _, value := range []float64{0, .1, 1, 10} {
		if !histogram.Add(value) {
			t.Fatalf("histogram rejected valid value %v", value)
		}
	}

	if value, ok := histogram.Quantile(.5); !ok || value != .5 {
		t.Fatalf("median=%v valid=%t, want conservative bucket 0.5", value, ok)
	}
	if value, ok := histogram.Quantile(1); !ok || value != 10 {
		t.Fatalf("maximum=%v valid=%t, want 10", value, ok)
	}
}

func TestDefaultLimitsBoundOneLiveSession(t *testing.T) {
	t.Parallel()

	limits := evidence.DefaultLimits()
	if limits.ChunkBytes != 400*1024 || limits.Chunks != 5 {
		t.Fatalf("default limits=%+v, want five 400 KiB chunks", limits)
	}
}
