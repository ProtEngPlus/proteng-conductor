// Package histogram counts assay scores into buckets for the Mutation Result chart
package histogram

import "math"

// BinCount is how many bars the chart has (frontend uses the same number)
const BinCount = 40

// Build picks the range from the data (min..max) and counts scores into buckets
// ok is false when there are no scores; equal scores get a widened range
func Build(scores []float64) (counts []int, min, max float64, ok bool) {
	if len(scores) == 0 {
		return nil, 0, 0, false
	}
	min, max = scores[0], scores[0]
	for _, s := range scores {
		if s < min {
			min = s
		}
		if s > max {
			max = s
		}
	}
	if min == max {
		min -= 1
		max += 1
	}
	return BuildInRange(scores, min, max), min, max, true
}

// BuildInRange counts scores into buckets over a range you pass in,
// so multiple collections of one job can share the same x-axis
func BuildInRange(scores []float64, min, max float64) []int {
	counts := make([]int, BinCount)
	width := (max - min) / float64(BinCount)
	if width <= 0 {
		return counts
	}
	for _, s := range scores {
		// which bucket, clamped so an edge value stays in range
		idx := int(math.Floor((s - min) / width))
		if idx < 0 {
			idx = 0
		}
		if idx >= BinCount {
			idx = BinCount - 1
		}
		counts[idx]++
	}
	return counts
}
