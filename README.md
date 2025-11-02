# Golang Implementation of the Feistel Network/Cipher
This library is an implementation of the [Feistel Cipher](https://en.wikipedia.org/wiki/Feistel_cipher) but not intended for cryptography. Think of it more like something that generates random permutations.

## How to use it

If you want to create something like [Perm in math/rand](https://pkg.go.dev/math/rand#Perm) you could do this:
```
import "github.com/elacy/feistel"

func feistelPerm(maxValue uint32, seed uint64)([]uint64, error){
    maxValue64 := uint64(maxValue)
    network, err := feistel.NewNetwork(maxValue64, seed, 8)

    if err != nil {
        return nil, err
    }

    values := make([]uint64, maxValue64+1)

    for i := range values{
        values[i], err = network.Map(i)

        if err != nil {
            return nil, err
        }
    }

    return values, nil
}

```


If you wanted to use it to do load balancing you could write something like this:

```

import (
        "encoding/binary"
        "fmt"
        "sync/atomic"
        "crypto/rand"

        "github.com/elacy/feistel"
)

func NewLoadBalancer[T any](values []T, seed uint64) (*LoadBalancer[T], error) {
        n := uint64(len(values))
        network, err := NewNetwork(n-1, seed, 8,  WithEpochs())

        if err != nil {
                return nil, err
        }

        return &LoadBalancer[T]{
                network: network,
                values:  values,
                n:       n,
        }, nil
}

type LoadBalancer[T any] struct {
        index   atomic.Uint64
        network *feistel.Network
        values  []T
        n       uint64
}

func (lb *LoadBalancer[T]) Next() (*T, error) {
        index := lb.index.Add(1) - 1
        position, err := lb.network.Map(index)

        if err != nil {
                return nil, err
        }

        position %= lb.n
        return &lb.values[position], nil
}

```

## What's unique about this implementation of Feistel?
- Instead of splitting the input number input parts and xoring a hash I'm generating factors and using them as radices to reduce the amount of cycle walking you have to do when the domain size isn't a power of 2
- I'm using SplitMix64 as a hash function because it's fast and it works
- This generates a unique seed per round to reduce the number of cyles and improve uniformity

## How did you test it?
All the testing that is being done is automated and included in the feistel_test.go file so you should be able to read that but for convenience I'll include the tests that are done here:
- Invertibility: If you Map a number, running InvertMap on it should bring you back to the start
- In Range: Values returned should be inside the range of the sequence
- Full Range Coverage: If I put the entire sequence in I should get the entire sequence out
- Cycle Lengths: Within a sequence if you map a number, and then map the output in a loop for each number you shouldn't get a lot of tiny cycles
- Uniform Distribution: Each member of the sequence should have an equal probability of appearing anywhere in the sequence  (only done for max size = 13/16)
- Fixed Points: Each Sequence should have on average 1 number that maps to itself
- Serial Correlation: Each Number should not be correlated with the number above it
- Serial Correlation across Epochs: Each number should not be correlated with the same position in the next epoch
- Serial Correlation across seeds: Each number should not be correlated with the same position in the next seed
- Unique Permutations: Each new permutation should be unique up until 1 millions mappings (only done for max size = 13/16)