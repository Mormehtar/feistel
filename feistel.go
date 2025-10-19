package feistel

import (
	"errors"
	"math/bits"

	"github.com/zeebo/xxh3"
)

// ErrRoundsAndAverageRoundsSet is returned when you try to set both rounds and minimum average rounds
var ErrRoundsAndAverageRoundsSet = errors.New("feistel: you cannot set both rounds and minimum average rounds")

// ErrIndexGreatThanMaxValue is returned when your index is greater than the maximum value
var ErrIndexGreatThanMaxValue = errors.New("feistel: index cannot be greater than max value")

// ErrRoundsMustBeSet is returned when you have set neither rounds nor minimum average rounds
var ErrRoundsMustBeSet = errors.New("feistel: you must set either rounds or minimumAverageRounds to some non zero value")

type Option func(*FeistelNetwork)

func WithRounds(number uint64) Option {
	return func(m *FeistelNetwork) {
		m.rounds = number
	}
}

func WithMinimumAverageRounds(number uint64) Option {
	return func(m *FeistelNetwork) {
		m.minAverageRounds = number
	}
}

func WithEpochs() Option {
	return func(m *FeistelNetwork) {
		m.epochs = true
	}
}

func NewNetwork(maxValue uint64, seed uint64, opts ...Option) (*FeistelNetwork, error) {
	bitWidth := uint64(bits.Len64(maxValue))

	if bitWidth%2 == 1 {
		bitWidth++
	}

	network := &FeistelNetwork{
		maxValue: maxValue,
		bitWidth: bitWidth,
		halfMask: (uint64(1) << (bitWidth / 2)) - 1,
		fullMask: (uint64(1) << bitWidth) - 1,
	}

	for _, opt := range opts {
		opt(network)
	}

	if network.rounds == 0 && network.minAverageRounds == 0 {
		return nil, ErrRoundsMustBeSet
	} else if network.rounds > 0 && network.minAverageRounds > 0 {
		return nil, ErrRoundsAndAverageRoundsSet
	}

	if maxValue == (uint64(1)<<bitWidth)-1 && network.minAverageRounds > 0 {
		network.rounds = network.minAverageRounds
	} else if network.minAverageRounds > 0 {
		hi, lo := bits.Mul64(network.minAverageRounds, network.maxValue+1)

		var quotient uint64
		var remainder uint64

		if network.bitWidth == 64 {
			quotient, remainder = hi, lo
		} else {
			quotient, remainder = bits.Div64(hi, lo, (uint64(1)<<network.bitWidth)-1)
		}

		if remainder > 0 {
			quotient++
		}
		network.requiredOutputsWithinMax = quotient
	}

	network.seeds = make([]uint64, network.rounds+network.requiredOutputsWithinMax)

	currentSeed := seed
	for i := range uint64(len(network.seeds)) {
		currentSeed = hashSeed(currentSeed, seed)
		network.seeds[i] = currentSeed
	}

	return network, nil
}

type FeistelNetwork struct {
	rounds           uint64
	minAverageRounds uint64
	maxValue         uint64
	roundCount       uint64
	epochs           bool
	seeds            []uint64

	bitWidth                 uint64
	halfMask                 uint64
	fullMask                 uint64
	requiredOutputsWithinMax uint64
}

func (n *FeistelNetwork) Map(index uint64) (uint64, error) {
	return n.encode(index, false)
}

func (n *FeistelNetwork) InvertMap(index uint64) (uint64, error) {
	return n.encode(index, true)
}

func (n *FeistelNetwork) hash(value uint64, seedId uint64) uint64 {
	return hashSeed(value, n.seeds[seedId])
}

func (n *FeistelNetwork) encode(index uint64, invert bool) (uint64, error) {
	var epochStart uint64
	domainSize := n.maxValue + 1

	if index > n.maxValue {
		if n.epochs {
			epochStart = index - (index % domainSize)
			index %= domainSize
		} else {
			return 0, ErrIndexGreatThanMaxValue
		}
	}

	if n.maxValue == 0 {
		return 0, nil
	}

	var withinMaxCount uint64

	left := index >> (n.bitWidth / 2)
	right := index & n.halfMask

	for {
		var seedId uint64
		if invert {
			if n.requiredOutputsWithinMax > 0 {
				seedId = n.requiredOutputsWithinMax - (withinMaxCount + 1)
			} else {
				seedId = n.rounds - ((n.roundCount % n.rounds) + 1)
			}
		} else {
			if n.requiredOutputsWithinMax > 0 {
				seedId = withinMaxCount
			} else {
				seedId = n.roundCount % n.rounds
			}
		}

		if invert {
			hashedLeft := n.hash(left+epochStart, seedId)

			hashedLeft = hashedLeft & n.halfMask
			left, right = right^hashedLeft, left

		} else {
			hashedRight := n.hash(right+epochStart, seedId)

			hashedRight = hashedRight & n.halfMask
			left, right = right, left^hashedRight
		}

		index = (left << (n.bitWidth / 2)) | right

		n.roundCount++

		if index <= n.maxValue {
			withinMaxCount++
		}

		finished := false

		if n.rounds > 0 && n.roundCount%n.rounds == 0 && index <= n.maxValue {
			finished = true
		} else if n.requiredOutputsWithinMax > 0 && withinMaxCount >= n.requiredOutputsWithinMax {
			finished = true
		}

		if finished {
			return index + epochStart, nil
		}
	}
}

func hashSeed(value uint64, seed uint64) uint64 {
	var buf [8]byte
	n := 0

	for value > 0 {
		buf[n] = byte(value)
		n++
		value >>= 8
	}

	return xxh3.HashSeed(buf[:n], seed)
}
