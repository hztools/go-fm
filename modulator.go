// {{{ Copyright (c) Paul R. Tagliamonte <paul@k3xec.com>, 2020
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE. }}}

package fm

import (
	"fmt"
	"math"

	"hz.tools/rf"
	"hz.tools/sdr"
)

const tau = math.Pi * 2

// EstimateBeta will
func EstimateBeta(desiredBandwidth rf.Hz, audioFrequency float64) float64 {
	return float64(desiredBandwidth) / audioFrequency
}

// ModulatorConfig is
type ModulatorConfig struct {
	// AudioSampleRate is the number of audio samples per second.
	AudioSampleRate uint

	// IqBufferLength is the amount of data to allocate to process incoming
	// Audio data.
	IqBufferLength uint

	// IqSamplesPerAudioSample controls how many Iq samples need to be generated
	// for each Audio sample that comes in.
	//
	// If the input AudioSampleRate is 44,100, and the IqSamplesPerAudioSample is
	// 10, the output SampleRate of the sdr.Reader will be 441,000.
	IqSamplesPerAudioSample uint

	// CarrierFrequency controls the frequency of the carrier that will be
	// modulated by incoming data.
	CarrierFrequency rf.Hz

	// Beta controls the deviation from the carrier based on the modulating
	// frequency.
	//
	// This value can get weird. If you want to estimate one, pick some values,
	// plug it into EstimateBeta, and go with it.
	Beta float64

	// Dest is where to send IQ samples to as audio data is written to the
	// Modulator.
	Dest sdr.Writer
}

// NewModulator allocates
func NewModulator(cfg ModulatorConfig) (*Modulator, error) {
	// TODO(paultag): do a sanity check:
	//
	// - check that everything has a valid value that makes sense and
	//   is non-zero.
	//
	// - check the sample rate of the Dest matches the AudioSampleRate and
	//   the IQ upscale rate.
	//
	// - check that the sample format is complex64.

	iqSampleRate := cfg.AudioSampleRate * cfg.IqSamplesPerAudioSample

	return &Modulator{
		Config:       cfg,
		iqSampleRate: uint(iqSampleRate),
		iqBuffer:     make(sdr.SamplesC64, cfg.IqBufferLength),
	}, nil
}

// Modulator is
type Modulator struct {
	// Config
	Config ModulatorConfig

	// iqSampleRate is the final samples per second of the samples written
	iqSampleRate uint

	// iqBuffer will be used when generating data to send to the Writer
	iqBuffer sdr.SamplesC64

	// timeOffset will be the number of samples processed. This over the number
	// of samples per second will return how many seconds of data have been
	// processed.
	timeOffset uint
}

// SampleRate implements the sdr.Writer interface.
func (m *Modulator) SampleRate() uint {
	return m.iqSampleRate
}

// Write accepts audio data as a set of audio samples as the provided
// Sample Rate, modulate them against the Carrier using Frequency Modulation,
// and write the IQ data to the resulting sdr.Writer
func (m *Modulator) Write(audioSamples []float32) (int, error) {
	iqBufLen := len(m.iqBuffer) / int(m.Config.IqSamplesPerAudioSample)

	var fn int
	for i := 0; i < len(audioSamples); i += iqBufLen {
		audioEnd := i + iqBufLen
		if audioEnd > len(audioSamples) {
			audioEnd = len(audioSamples)
		}

		n, err := m.write(audioSamples[i:audioEnd])
		if err != nil {
			return n, err
		}
		fn += n

		if n != (audioEnd - i) {
			return fn, fmt.Errorf("fmtx.Write: incomplete write call")
		}
	}
	return fn, nil
}

// perform the actual write
func (m *Modulator) write(audioSamples []float32) (int, error) {
	iqPerA := int(m.Config.IqSamplesPerAudioSample)

	if len(m.iqBuffer) < len(audioSamples)*iqPerA {
		return 0, fmt.Errorf("fmtx.Write: iq buffer is too short for audio buffer")
	}

	timeOffset := float64(m.timeOffset)
	beta := m.Config.Beta

	for audioStep := range audioSamples {
		var (
			audioSample = float64(audioSamples[audioStep])
			iqStepStart = audioStep * iqPerA
			iqStepEnd   = iqStepStart + iqPerA
		)

		for iqStep := iqStepStart; iqStep < iqStepEnd; iqStep++ {
			var (
				now        = timeOffset / float64(m.iqSampleRate)
				realSample = math.Cos(tau*float64(m.Config.CarrierFrequency)*now + beta*audioSample)
				imagSample = math.Sin(tau*float64(m.Config.CarrierFrequency)*now + beta*audioSample)
			)
			m.iqBuffer[iqStep] = complex(float32(realSample), float32(imagSample))
			timeOffset = (timeOffset + 1)
		}
	}

	expectedSamples := len(audioSamples) * int(iqPerA)

	n, err := m.Config.Dest.Write(m.iqBuffer[:expectedSamples])
	if err != nil {
		return n / iqPerA, err
	}

	if n != expectedSamples {
		return n / iqPerA, fmt.Errorf("fmtx.Write: i wrote a bad count, %d vs %d", n, expectedSamples)
	}

	timeTicks := uint(timeOffset) - m.timeOffset
	if timeTicks != uint(expectedSamples) {
		return n / iqPerA, fmt.Errorf("fmtx.Write: timeTick mismatch %d vs %d", timeTicks, expectedSamples)
	}

	m.timeOffset = uint(timeOffset)
	return expectedSamples / iqPerA, err
}

// vim: foldmethod=marker
