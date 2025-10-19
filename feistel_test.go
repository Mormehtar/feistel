package feistel

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/gonum/stat/distuv"
)

/*
Left to do:
- Use Seed to create more data for tests
- Fixed Points should only be used where Distribution validation takes too much time
- Ensure all tests cover all results they can
- Cycle Length expanded to cover epochs
- See if we can remove the min average rounds somehow
- Reduce repeating code
- Include more domain sizes
- Make sure you understand everything in the test suite
- Ask chatgpt if there is anything you've left out in the tests
- Validate that this will actually work
*/

const maxTestRange = 1000000

type testSettings struct {
	minAverageRounds uint64
	maxValue         uint64
	rounds           uint64
	startIndex       uint64
	endIndex         uint64
	epochs           uint64
}

func (s testSettings) String() string {
	descriptors := []string{
		fmt.Sprintf("maxValue %d, range: %d -> %d", s.maxValue, s.startIndex, s.endIndex),
	}

	if s.minAverageRounds > 0 {
		descriptors = append(descriptors, fmt.Sprintf("minAverageRounds: %d", s.minAverageRounds))
	} else if s.rounds > 0 {
		descriptors = append(descriptors, fmt.Sprintf("rounds: %d", s.rounds))
	}

	return strings.Join(descriptors, ", ")
}

type testResult struct {
	settings    *testSettings
	forwardMap  map[uint64]uint64
	reverseMap  map[uint64]uint64
	totalRounds uint64
	err         error
}

var maxValuesToTest = []uint64{13, 16, 64, 101, 1000, 1024, 50_000}
var optionsToTest = []testSettings{
	{
		minAverageRounds: 4,
	},
	{
		minAverageRounds: 8,
	},
	{
		minAverageRounds: 2,
	},
	{
		minAverageRounds: 1,
	},
	{
		rounds: 1,
	},
	{
		rounds: 8,
	},
	{
		rounds: 30,
	},
}

var cachedResults []testResult

func TestMain(m *testing.M) {
	cachedResults = make([]testResult, len(maxValuesToTest)*len(optionsToTest)*2)

	i := 0

	for _, epochs := range []bool{false, true} {
		for _, maxValue := range maxValuesToTest {
			for _, settings := range optionsToTest {
				settings.maxValue = maxValue
				if epochs {
					settings.epochs = maxTestRange / maxValue
				} else {
					settings.epochs = 1
				}
				cachedResults[i] = runTest(settings)
				i++
			}
		}
	}

	os.Exit(m.Run())
}

func runTest(s testSettings) testResult {
	settings := &s
	var options []Option

	result := testResult{
		settings: settings,
	}

	if settings.minAverageRounds > 0 {
		options = append(options, WithMinimumAverageRounds(settings.minAverageRounds))
	} else if settings.rounds > 0 {
		options = append(options, WithRounds(settings.rounds))
	}

	if settings.epochs > 1 {
		options = append(options, WithEpochs())
		settings.endIndex = ((settings.maxValue + 1) * settings.epochs) - 1
	}

	if settings.endIndex == 0 {
		settings.endIndex = settings.maxValue
	}

	net, err := NewNetwork(settings.maxValue, 0, options...)

	if err != nil {
		result.err = err
		result.settings = settings
		return result
	}

	result.forwardMap = make(map[uint64]uint64, settings.endIndex+1-settings.startIndex)
	result.reverseMap = make(map[uint64]uint64, settings.endIndex+1-settings.startIndex)

	for i := settings.startIndex; i <= settings.endIndex; i++ {
		mappedValue, err := net.Map(i)

		if err != nil {
			result.err = fmt.Errorf("Error Mapping %d, %w", i, err)
			return result
		}

		result.forwardMap[i] = mappedValue
		result.reverseMap[mappedValue], err = net.InvertMap(mappedValue)

		if err != nil {
			result.err = fmt.Errorf("Error inverting %d, %w", i, err)
			return result
		}
	}

	result.totalRounds = net.roundCount

	return result
}

func TestValuesWithinRange(t *testing.T) {
	for _, test := range cachedResults {
		t.Run(test.settings.String(), func(t *testing.T) {
			for original, value := range test.forwardMap {
				if test.settings.epochs > 1 {
					domainSize := test.settings.maxValue + 1
					start := original - (original % domainSize)

					if value < start {
						t.Errorf("Epoch mapped value %d is below start index %d", value, start)
					} else if value > start+test.settings.maxValue {
						t.Errorf("Epoch mapped value %d is above end index %d", value, start+test.settings.maxValue)
					}
				} else if value > test.settings.maxValue {
					t.Errorf("Forward map contains the value %d which exceeds its max value of %d", value, test.settings.maxValue)
				}
			}
		})
	}
}

func TestInvertible(t *testing.T) {
	for _, test := range cachedResults {
		t.Run(test.settings.String(), func(t *testing.T) {
			for original, mappedValue := range test.forwardMap {
				if inverted, ok := test.reverseMap[mappedValue]; ok {
					if original != inverted {
						t.Errorf("Mapped %d to %d and inversion produced %d", original, mappedValue, inverted)
					}
				} else {
					t.Errorf("Inversion not performed for %d which mapped to %d", original, mappedValue)
				}
			}
		})
	}
}

func TestNoErr(t *testing.T) {
	for _, test := range cachedResults {
		t.Run(test.settings.String(), func(t *testing.T) {
			if test.err != nil {
				t.Errorf("Error occured %v", test.err)
			}
		})
	}
}

func TestAverageFixedPoints(t *testing.T) {
	total := uint64(0)
	fixedPoints := 0
	zeroFixedPoints := 0

	for _, test := range cachedResults {
		if test.settings.maxValue < 20 {
			continue
		}

		if test.settings.rounds < 3 && test.settings.minAverageRounds < 9 {
			continue
		}

		total += test.settings.epochs

		for original, mapped := range test.forwardMap {
			if original == mapped {
				fixedPoints++

				if original == 0 {
					zeroFixedPoints++
				}
			}
		}
	}

	average := float64(fixedPoints) / float64(total)

	if average > 1 || average < 0.02 {
		t.Errorf("Average fixed points not in valid range (between 0.02 and 1) total %d, fixed points %d, average %f", total, fixedPoints, average)
	}

	if fixedPoints/2 < zeroFixedPoints {
		t.Errorf("High number of zero fixed points %d", zeroFixedPoints)
	}
}

func TestFullRangeCovered(t *testing.T) {
	for _, test := range cachedResults {
		if test.settings.endIndex != test.settings.maxValue || test.settings.startIndex != 0 {
			continue
		}
		t.Run(test.settings.String(), func(t *testing.T) {
			seen := make(map[uint64]uint64, test.settings.maxValue+1)

			for original, mapped := range test.forwardMap {
				if previous, ok := seen[mapped]; ok {
					t.Errorf("both %d and %d are mapped to %d in forward map", previous, original, mapped)
					return
				}

				seen[mapped] = original
			}

			for i := range test.settings.maxValue + 1 {
				if _, ok := seen[i]; !ok {
					t.Errorf("%d was not covered in forward map", i)
					return
				}
			}
		})
	}
}

func TestAverage(t *testing.T) {
	for _, test := range cachedResults {
		if test.settings.minAverageRounds == 0 {
			continue
		}
		t.Run(test.settings.String(), func(t *testing.T) {
			avgRounds := test.totalRounds / (uint64(len(test.forwardMap)) * 2)
			if avgRounds < test.settings.minAverageRounds-1 {
				t.Errorf("Average Rounds too low, expected >%d received %d, %d values", test.settings.minAverageRounds, avgRounds, len(test.forwardMap))
			}

			if avgRounds > test.settings.minAverageRounds+3 {
				t.Errorf("Average Rounds too high, expected <%d+4 received %d, %d values", test.settings.minAverageRounds, avgRounds, len(test.forwardMap))
			}
		})
	}
}

func TestCycleLengths(t *testing.T) {
	for _, test := range cachedResults {
		if (test.settings.rounds < 3 && test.settings.minAverageRounds < 5) || test.settings.endIndex != test.settings.maxValue || test.settings.startIndex != 0 {
			continue
		}
		t.Run(test.settings.String(), func(t *testing.T) {
			var cycles []int
			cycleLength := 0
			seen := make(map[uint64]struct{}, len(test.forwardMap))

			for currentValue := range test.forwardMap {
				if _, ok := seen[currentValue]; ok {
					continue
				}
				for {
					seen[currentValue] = struct{}{}
					cycleLength++
					currentValue = test.forwardMap[currentValue]

					if _, ok := seen[currentValue]; ok {
						cycles = append(cycles, cycleLength)
						cycleLength = 0
						break
					}
				}
			}

			maxCycles := maxAllowedCycles(len(test.forwardMap), 3)
			if len(cycles) > maxCycles {
				t.Errorf("Number of cycles (%d) exceeds max %d for n %d", len(cycles), maxCycles, len(test.forwardMap))
				println(fmt.Sprintf("Cycles are %v", cycles))
			}
		})
	}
}

// eulerMascheroni is γ ≈ 0.5772156649
const eulerMascheroni = 0.5772156649015329

// MaxAllowedCycles returns ceil( ln(n) + γ + z*sqrt(ln(n)) ).
// Use z=3 for a lenient (~3σ) upper bound. n is the domain size (count of elements), not the max index.
func maxAllowedCycles(n int, z float64) int {
	if n < 2 {
		// For n=0 or n=1, the only possible permutation has exactly 0 or 1 cycle respectively.
		return int(n)
	}
	ln := math.Log(float64(n))
	mean := ln + eulerMascheroni
	sd := math.Sqrt(ln)
	return int(math.Ceil(mean + z*sd))
}

func TestChiSquare(t *testing.T) {
	for _, test := range cachedResults {
		if (test.settings.rounds < 3 && test.settings.minAverageRounds < 5) || len(test.forwardMap) < 1000 {
			continue
		}
		t.Run(test.settings.String(), func(t *testing.T) {
			mappedValues := mapValues(test.forwardMap)
			hist := histogram(mappedValues[0:len(mappedValues)/2], test.settings.maxValue+1, 64)
			chiSquaredTest(t, hist, 0.1)
		})
	}
}

func histogram(samples []uint64, n uint64, bins uint64) []uint64 {
	hist := make([]uint64, bins)
	for _, v := range samples {
		if v >= n {
			continue // skip invalid
		}
		b := (uint64(bins) * v) / n
		if b >= bins {
			b = bins - 1
		}
		hist[b]++
	}
	return hist
}

func chiSquaredTest(t *testing.T, values []uint64, threshold float64) {
	t.Helper()

	floatValues := castToFloat64(values)
	mean := stat.Mean(floatValues, nil)
	averages := repeat(mean, len(values))
	chi2 := stat.ChiSquare(floatValues, averages)

	p := distuv.ChiSquared{K: float64(len(values) - 1)}.Survival(chi2)

	if threshold > p {
		t.Errorf("Unven Distribution, p is %f, values: %v", p, values)
	}
}

func castToFloat64(values []uint64) []float64 {
	result := make([]float64, len(values))

	for i := range len(values) {
		result[i] = float64(values[i])
	}

	return result
}

func repeat[T any](value T, count int) []T {
	result := make([]T, count)

	for i := range count {
		result[i] = value
	}

	return result
}

func mapValues[K comparable, V any](input map[K]V) []V {
	result := make([]V, len(input))

	i := 0

	for _, value := range input {
		result[i] = value
		i++
	}

	return result
}

func TestSerialCorrelationLag1(t *testing.T) {
	for _, test := range cachedResults {
		if test.settings.startIndex != 0 || test.settings.endIndex != test.settings.maxValue {
			continue
		}

		if test.settings.rounds < 3 && test.settings.minAverageRounds < 5 {
			continue
		}

		t.Run(test.settings.String(), func(t *testing.T) {
			pairCount := test.settings.maxValue
			current := make([]float64, pairCount)
			next := make([]float64, pairCount)

			for i := range pairCount {
				current[i] = float64(test.forwardMap[i])
				next[i] = float64(test.forwardMap[i+1])
			}

			corr := stat.Correlation(current, next, nil)
			if math.IsNaN(corr) {
				t.Fatal("correlation is NaN")
			}

			// Expected mean correlation for permutations (slight negative).
			expected := -1.0 / float64(pairCount)

			// Fisher z transform: z = atanh(r); SE ≈ 1/sqrt(n-3)
			// Compare (z - z_expected) to 3σ.
			z := 0.5 * math.Log((1+corr)/(1-corr))
			zExp := 0.5 * math.Log((1+expected)/(1-expected))
			se := 1.0 / math.Sqrt(float64(pairCount-2))
			zScore := (z - zExp) / se

			if math.Abs(zScore) > 3.0 {
				t.Errorf("lag-1 correlation out of band: corr=%.5f, z=%.2f (expected≈%.5f, SE≈%.3f)",
					corr, zScore, expected, se)
			}
		})
	}
}

func TestResidueBiasSmallPrimes(t *testing.T) {
	for _, test := range cachedResults {
		// Only meaningful when we've mapped the full domain [0..maxValue].
		if test.settings.startIndex != 0 || test.settings.endIndex != test.settings.maxValue {
			continue
		}

		t.Run(test.settings.String(), func(t *testing.T) {
			outputs := mapValues(test.forwardMap)

			significance := 0.01
			primes := []uint64{3, 5, 7, 11, 13, 17, 19, 23}

			for _, p := range primes {
				// Ensure enough mass so expected count per residue ≥ ~20
				if len(outputs) < int(p*20) {
					continue
				}
				// Count residues of y mod p across all outputs.
				counts := histogram(outputs, p, p) // your helper: n=p, bins=p
				chiSquaredTest(t, counts, significance)
			}
		})
	}
}

func TestUniqueCombinations(t *testing.T) {
	for _, test := range cachedResults {
		// Only meaningful when we've mapped the full domain [0..maxValue].
		if test.settings.epochs < 2 {
			continue
		}

		if test.settings.rounds < 3 && test.settings.minAverageRounds < 5 {
			continue
		}
		t.Run(test.settings.String(), func(t *testing.T) {
			seen := make(map[string]struct{}, test.settings.epochs)
			domainSize := test.settings.maxValue + 1
			buf := make([]byte, 8*domainSize)

			for i := range uint64(len(test.forwardMap)) {
				position := i % domainSize
				value := test.forwardMap[i] % domainSize

				bufStart := position * 8
				binary.BigEndian.PutUint64(buf[bufStart:bufStart+8], value)

				if position == domainSize-1 {
					seen[string(buf)] = struct{}{}
				}
			}

			outputs := mapValues(test.forwardMap)

			significance := 0.01
			primes := []uint64{3, 5, 7, 11, 13, 17, 19, 23}

			for _, p := range primes {
				// Ensure enough mass so expected count per residue ≥ ~20
				if len(outputs) < int(p*20) {
					continue
				}
				// Count residues of y mod p across all outputs.
				counts := histogram(outputs, p, p) // your helper: n=p, bins=p
				chiSquaredTest(t, counts, significance)
			}

			if len(seen) < int(test.settings.epochs) {
				t.Errorf("Total combinations found was %d but total epochs was %d", len(seen), test.settings.epochs)
			}
		})
	}
}

func TestSerialEpochCorrelationLag1(t *testing.T) {
	for _, test := range cachedResults {
		if test.settings.rounds < 3 && test.settings.minAverageRounds < 5 {
			continue
		}

		if test.settings.epochs < 2 {
			continue
		}

		t.Run(test.settings.String(), func(t *testing.T) {
			domainSize := test.settings.maxValue + 1
			pairCount := test.settings.epochs - 1
			current := make([]float64, pairCount)
			next := make([]float64, pairCount)

			for i := range pairCount {
				current[i] = float64(test.forwardMap[i*domainSize] % domainSize)
				next[i] = float64(test.forwardMap[(i+1)*domainSize] % domainSize)
			}

			corr := stat.Correlation(current, next, nil)
			if math.IsNaN(corr) {
				t.Fatal("correlation is NaN")
			}

			// Expected mean correlation for permutations (slight negative).
			expected := -1.0 / float64(pairCount)

			// Fisher z transform: z = atanh(r); SE ≈ 1/sqrt(n-3)
			// Compare (z - z_expected) to 3σ.
			z := 0.5 * math.Log((1+corr)/(1-corr))
			zExp := 0.5 * math.Log((1+expected)/(1-expected))
			se := 1.0 / math.Sqrt(float64(pairCount-2))
			zScore := (z - zExp) / se

			if math.Abs(zScore) > 3.0 {
				t.Errorf("lag-1 correlation out of band: corr=%.5f, z=%.2f (expected≈%.5f, SE≈%.3f)",
					corr, zScore, expected, se)
			}
		})
	}
}

func TestEpochDistribution(t *testing.T) {
	for _, test := range cachedResults {
		if test.settings.rounds < 3 && test.settings.minAverageRounds < 9 {
			continue
		}

		if test.settings.maxValue > 20 {
			continue
		}

		if test.settings.epochs < 2 {
			continue
		}

		t.Run(test.settings.String(), func(t *testing.T) {
			domainSize := test.settings.maxValue + 1
			counts := make([]uint64, domainSize*domainSize)

			for original, mapped := range test.forwardMap {
				original := original % domainSize
				mapped := mapped % domainSize
				index := (original * domainSize) + mapped
				counts[index] += 1
			}

			chiSquaredTest(t, counts, 0.1)
		})
	}
}
