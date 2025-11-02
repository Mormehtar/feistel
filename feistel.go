package feistel

import (
	"errors"
	"math"
)

// ErrIndexGreatThanMaxValue is returned when your index is greater than the maximum value
var ErrIndexGreatThanMaxValue = errors.New("feistel: index cannot be greater than max value")

// ErrRoundsMustBeSet is returned when you have provided a zero value for rounds
var ErrRoundsMustBeSet = errors.New("feistel: rounds must be a non zero value")

type Option func(*FeistelNetwork)

func WithEpochs() Option {
	return func(m *FeistelNetwork) {
		m.epochs = true
	}
}

func NewNetwork(maxValue, seed uint64, rounds uint8, opts ...Option) (*FeistelNetwork, error) {
	l, r := findFactors(maxValue)

	network := &FeistelNetwork{
		maxValue:   maxValue,
		rounds:     int(rounds),
		leftRadix:  l,
		rightRadix: r,
	}

	for _, opt := range opts {
		opt(network)
	}

	if network.rounds == 0 {
		return nil, ErrRoundsMustBeSet
	}

	network.seeds = make([]uint64, network.rounds)

	currentSeed := seed
	for i := range uint64(len(network.seeds)) {
		currentSeed = splitmix64(currentSeed)
		network.seeds[i] = currentSeed
	}

	return network, nil
}

type FeistelNetwork struct {
	rounds   int
	maxValue uint64
	epochs   bool
	seeds    []uint64

	leftRadix  uint64
	rightRadix uint64
}

func (n *FeistelNetwork) Map(index uint64) (uint64, error) {
	return n.encode(index, false)
}

func (n *FeistelNetwork) InvertMap(index uint64) (uint64, error) {
	return n.encode(index, true)
}

func (n *FeistelNetwork) encode(index uint64, invert bool) (uint64, error) {
	var epochStart uint64
	var epochHash uint64
	domainSize := n.maxValue + 1

	if index > n.maxValue {
		if n.epochs {
			epochStart = index - (index % domainSize)
			epochHash = splitmix64(epochStart)
			index %= domainSize
		} else {
			return 0, ErrIndexGreatThanMaxValue
		}
	}

	if n.maxValue == 0 {
		return 0, nil
	}

	a := index % n.leftRadix
	b := index / n.leftRadix

	for {
		start := 0
		adjust := 1

		if invert {
			start = n.rounds - 1
			adjust = -1
		}

		for round := start; round >= 0 && round < n.rounds; round += adjust {
			seed := n.seeds[round]

			if round%2 == 0 {
				f := splitmix64(b^epochHash^seed) % n.leftRadix
				if invert {
					a = (a + n.leftRadix - f) % n.leftRadix
				} else {
					a = (a + f) % n.leftRadix
				}
			} else {
				f := splitmix64(a^epochHash^seed) % n.rightRadix
				if invert {
					b = (b + n.rightRadix - f) % n.rightRadix
				} else {
					b = (b + f) % n.rightRadix
				}
			}
		}

		index = a + b*n.leftRadix

		if index <= n.maxValue {
			return index + epochStart, nil
		}
	}
}

func findFactors(maxValue uint64) (uint64, uint64) {
	if maxValue < 3 {
		return 2, 2
	}

	if maxValue == ^uint64(0) {
		val := uint64(1) << 32
		return val, val
	}

	value := maxValue + 1
	sqrt := uint64CeilSqrt(value)

	var bestValue uint64
	var bestOther uint64
	var exceeds uint64

	for current := sqrt; (current*2) > (value/current) && current > 1; current-- {
		if value%current == 0 {
			return current, value / current
		}

		other := (value / current) + 1
		diff := (other * current) - value

		if bestValue == 0 || exceeds > diff {
			exceeds = diff
			bestOther = other
			bestValue = current
		}
	}

	return bestValue, bestOther
}

func uint64CeilSqrt(n uint64) uint64 {
	if n < 2 {
		return n
	}
	x := uint64(math.Sqrt(float64(n)))

	for x > 0 && x-1 > n/x {
		x--
	}
	for x < n/x {
		x++
	}
	return x
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
