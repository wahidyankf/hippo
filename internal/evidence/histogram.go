package evidence

import "math"

// Histogram records bounded finite values at a fixed resolution.
type Histogram struct {
	resolution float64
	counts     []uint64
	total      uint64
}

// NewHistogram creates a histogram covering zero through maximum, inclusive.
func NewHistogram(maximum, resolution float64) *Histogram {
	if math.IsNaN(maximum) || math.IsInf(maximum, 0) || maximum < 0 ||
		math.IsNaN(resolution) || math.IsInf(resolution, 0) || resolution <= 0 ||
		maximum/resolution > float64(math.MaxInt-1) {
		return &Histogram{}
	}

	buckets := int(math.Ceil(maximum/resolution)) + 1

	return &Histogram{
		resolution: resolution,
		counts:     make([]uint64, buckets),
	}
}

// Add records one finite in-range value and reports whether it was accepted.
// Values are rounded upward so threshold evidence remains conservative.
func (histogram *Histogram) Add(value float64) bool {
	if histogram == nil || histogram.resolution <= 0 || len(histogram.counts) == 0 ||
		math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return false
	}

	index := int(math.Ceil(value / histogram.resolution))
	if index >= len(histogram.counts) {
		return false
	}

	histogram.counts[index]++
	histogram.total++

	return true
}

// Quantile returns the nearest-rank quantile for a finite proportion in (0, 1].
func (histogram *Histogram) Quantile(proportion float64) (float64, bool) {
	if histogram == nil || histogram.total == 0 || math.IsNaN(proportion) || math.IsInf(proportion, 0) || proportion <= 0 || proportion > 1 {
		return 0, false
	}

	rank := uint64(math.Ceil(float64(histogram.total) * proportion))
	var cumulative uint64
	selected := 0

	for index, count := range histogram.counts {
		cumulative += count
		if cumulative >= rank {
			selected = index

			break
		}
	}

	return float64(selected) * histogram.resolution, true
}
