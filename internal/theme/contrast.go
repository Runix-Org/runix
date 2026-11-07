package theme

import (
	"math"
	"slices"
)

// SearchTolerance bounds interpolation/opacity error, not display-byte rounding.
// RatioTolerance is used only for floating-point feasibility at a solved boundary.
const (
	SearchTolerance = 1e-10
	RatioTolerance  = 1e-9
	contrastTie     = 1e-12
	fallbackSteps   = 256
)

type contrastSample struct {
	BackgroundSample
	luminance float64
}

type contrastEvaluator struct {
	samples  []contrastSample
	contexts []Context
}

func newEvaluator(samples []BackgroundSample) contrastEvaluator {
	// Exact-color deduplication avoids repeating flat roots, popups and clamped gradient stops.
	seen := make(map[Color]bool, len(samples))
	contexts := make(map[Context]bool)
	result := contrastEvaluator{samples: make([]contrastSample, 0, len(samples))}
	for _, sample := range samples {
		if !contexts[sample.Context] {
			contexts[sample.Context] = true
			result.contexts = append(result.contexts, sample.Context)
		}
		if !seen[sample.Color] {
			seen[sample.Color] = true
			result.samples = append(result.samples, contrastSample{sample, luminance(sample.Color)})
		}
	}
	slices.SortStableFunc(result.samples, func(a, b contrastSample) int {
		if a.luminance < b.luminance {
			return -1
		}
		if a.luminance > b.luminance {
			return 1
		}
		return 0
	})
	return result
}

func (e contrastEvaluator) minimum(color Color) (float64, BackgroundSample) {
	minimum, worst := math.Inf(1), BackgroundSample{}
	if color.A == 1 {
		value := luminance(color)
		index, _ := slices.BinarySearchFunc(e.samples, value, func(sample contrastSample, target float64) int {
			if sample.luminance < target {
				return -1
			}
			if sample.luminance > target {
				return 1
			}
			return 0
		})
		// Only the two nearest background luminances can determine the opaque minimum.
		for _, candidate := range [...]int{index - 1, index} {
			if candidate >= 0 && candidate < len(e.samples) {
				sample := e.samples[candidate]
				ratio := contrastLuminance(value, sample.luminance)
				if ratio < minimum {
					minimum, worst = ratio, sample.BackgroundSample
				}
			}
		}
		return minimum, worst
	}
	for _, sample := range e.samples {
		ratio := contrastLuminance(luminance(color.Over(sample.Color)), sample.luminance)
		if ratio < minimum {
			minimum, worst = ratio, sample.BackgroundSample
		}
	}
	return minimum, worst
}

func deriveForeground(name string, original Color, target float64,
	evaluator contrastEvaluator, improve bool,
) Foreground {
	effective := original
	minimum, _ := evaluator.minimum(original)
	if improve && target > 0 && minimum+RatioTolerance < target {
		effective = evaluator.correct(original, target)
	}
	minimum, worst := evaluator.minimum(effective)
	return Foreground{
		Name: name, Original: original, Effective: effective, Contexts: evaluator.contexts,
		Diagnostic: Diagnostic{
			Minimum: minimum, Target: target, Worst: worst,
			Passing: target == 0 || minimum+RatioTolerance >= target, Changed: effective != original,
		},
	}
}

// forbidden describes an open interval of path amounts failing one background.
type forbidden struct{ start, end float64 }

// firstPassing solves all sampled backgrounds together, rather than assuming that the
// minimum contrast is monotone. Encoded-sRGB luminance along any affine RGB path is convex.
// Split at its minimum, solve each monotone segment's forbidden luminance band, then union
// those bands. This also finds narrow feasible gaps that an interpolation grid can miss.
func (e contrastEvaluator) firstPassing(path func(float64) Color, target float64) (float64, bool) {
	intervals := make([]forbidden, 0, len(e.samples)*2)
	for _, sample := range e.samples {
		value := func(amount float64) float64 { return luminance(path(amount).Over(sample.Color)) }
		low, high := 0.0, 1.0
		// Convex ternary search also handles monotone and constant paths.
		for high-low > SearchTolerance {
			left, right := low+(high-low)/3, high-(high-low)/3
			if value(left) <= value(right) {
				high = right
			} else {
				low = left
			}
		}
		middle := (low + high) / 2
		lower := (sample.luminance+0.05)/target - 0.05
		upper := target*(sample.luminance+0.05) - 0.05
		for _, segment := range [...][2]float64{{0, middle}, {middle, 1}} {
			start, end := segment[0], segment[1]
			a, b := value(start), value(end)
			if math.Max(a, b) <= lower || math.Min(a, b) >= upper {
				continue
			}
			increasing := b >= a
			boundary := func(level float64) float64 {
				lo, hi := start, end
				for hi-lo > SearchTolerance {
					mid := (lo + hi) / 2
					if (value(mid) < level) == increasing {
						lo = mid
					} else {
						hi = mid
					}
				}
				return (lo + hi) / 2
			}
			if increasing {
				if a < lower {
					start = boundary(lower)
				}
				if b > upper {
					end = boundary(upper)
				}
			} else {
				if a > upper {
					start = boundary(upper)
				}
				if b < lower {
					end = boundary(lower)
				}
			}
			intervals = append(intervals, forbidden{start, end})
		}
	}
	slices.SortFunc(intervals, func(a, b forbidden) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return 0
	})
	candidate := 0.0
	for _, interval := range intervals {
		if interval.start > candidate+SearchTolerance {
			if minimum, _ := e.minimum(path(candidate)); minimum+RatioTolerance >= target {
				return candidate, true
			}
		}
		if interval.start <= candidate+SearchTolerance && interval.end > candidate {
			candidate = interval.end
		}
	}
	minimum, _ := e.minimum(path(candidate))
	return candidate, minimum+RatioTolerance >= target
}

// opaquePassing sweeps ordered forbidden luminance bands, then inverts the
// monotone black/white interpolation once instead of scanning every RGB path.
func (e contrastEvaluator) opaquePassing(original, endpoint Color, target float64) (float64, bool) {
	value := luminance(original)
	darkening := endpoint.R == 0
	for step := range len(e.samples) {
		index := step
		if darkening {
			index = len(e.samples) - 1 - step
		}
		background := e.samples[index].luminance
		lower := (background+0.05)/target - 0.05
		upper := target*(background+0.05) - 0.05
		if value > lower && value < upper {
			if darkening {
				value = lower
			} else {
				value = upper
			}
		}
	}
	if value < 0 || value > 1 {
		return 0, false
	}
	low, high := 0.0, 1.0
	for high-low > SearchTolerance {
		middle := (low + high) / 2
		current := luminance(interpolate(original, endpoint, middle))
		if (current > value) == darkening {
			low = middle
		} else {
			high = middle
		}
	}
	minimum, _ := e.minimum(interpolate(original, endpoint, high))
	return high, minimum+RatioTolerance >= target
}

func (e contrastEvaluator) correct(original Color, target float64) Color {
	if original.A < 1 {
		path := func(amount float64) Color {
			return original.WithAlpha(original.A + (1-original.A)*amount)
		}
		if amount, ok := e.firstPassing(path, target); ok {
			return path(amount)
		}
	}
	opaque := original.WithAlpha(1)
	best, bestAmount, found := original, math.Inf(1), false
	for _, endpoint := range [...]Color{{A: 1}, {R: 1, G: 1, B: 1, A: 1}} {
		path := func(amount float64) Color { return interpolate(opaque, endpoint, amount) }
		if amount, ok := e.opaquePassing(opaque, endpoint, target); ok {
			if !found || amount < bestAmount-SearchTolerance {
				best, bestAmount, found = path(amount), amount, true
			}
		}
	}
	if found {
		return best
	}
	bestMinimum, _ := e.minimum(best)
	// Only impossible cases need a bounded best-effort grid. Preserve original
	// on equal minima, then prefer opacity, black and white paths in that order.
	paths := [][2]Color{{opaque, Color{A: 1}}, {opaque, Color{1, 1, 1, 1}}}
	if original.A < 1 {
		paths = append([][2]Color{{original, opaque}}, paths...)
	}
	for _, path := range paths {
		for index := range fallbackSteps + 1 {
			candidate := interpolate(path[0], path[1], float64(index)/fallbackSteps)
			minimum, _ := e.minimum(candidate)
			if minimum > bestMinimum+contrastTie {
				best, bestMinimum = candidate, minimum
			}
		}
	}
	return best
}
