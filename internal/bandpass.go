package internal

import (
	"hz.tools/rf"
	"hz.tools/sdr/fft"
)

// Filter will design a super hacky filter in frequency space.
func Filter(
	dst []complex64,
	sampleRate uint,
	order fft.Order,
	cf rf.Hz,
	dv rf.Hz,
) error {
	bins, err := fft.BinsByRange(dst, sampleRate, order, rf.Range{cf - dv, cf + dv})
	if err != nil {
		return err
	}

	for _, idx := range bins {
		dst[idx] = complex64(complex(1, 0))
	}

	return nil
}
