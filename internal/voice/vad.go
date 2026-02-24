package voice

import (
	"math"
	"time"
)

// VADEvent represents a voice activity detection event
type VADEvent int

const (
	VADSilence       VADEvent = iota // No speech detected
	VADSpeechStarted                 // Speech just started
	VADSpeechActive                  // Speech is ongoing
	VADSpeechEnded                   // Speech just ended (silence after speech)
)

// VAD performs simple voice activity detection using RMS energy threshold
type VAD struct {
	threshold       int16         // RMS threshold to consider as speech
	silenceTimeout  time.Duration // Duration of silence before declaring speech end
	isSpeaking      bool          // Whether we're currently in a speech segment
	lastSpeechTime  time.Time     // When we last detected speech
	speechStartTime time.Time     // When current speech segment started
}

// NewVAD creates a new Voice Activity Detector
// threshold: RMS energy threshold (recommended: 500-2000)
// silenceTimeout: how long to wait after last speech before ending (recommended: 1.5s)
func NewVAD(threshold int16, silenceTimeout time.Duration) *VAD {
	return &VAD{
		threshold:      threshold,
		silenceTimeout: silenceTimeout,
	}
}

// Process analyzes an audio chunk and returns the current VAD event
func (v *VAD) Process(data []byte) VADEvent {
	rms := computeRMS(data)
	now := time.Now()

	if rms > v.threshold {
		// Audio is above threshold — speech detected
		v.lastSpeechTime = now

		if !v.isSpeaking {
			v.isSpeaking = true
			v.speechStartTime = now
			return VADSpeechStarted
		}
		return VADSpeechActive
	}

	// Audio is below threshold — silence
	if v.isSpeaking {
		// Check if we've been silent long enough to end speech
		if now.Sub(v.lastSpeechTime) >= v.silenceTimeout {
			v.isSpeaking = false
			return VADSpeechEnded
		}
		// Still within silence timeout — consider speech ongoing
		return VADSpeechActive
	}

	return VADSilence
}

// IsSpeaking returns whether the VAD currently detects speech
func (v *VAD) IsSpeaking() bool {
	return v.isSpeaking
}

// Reset clears the VAD state
func (v *VAD) Reset() {
	v.isSpeaking = false
	v.lastSpeechTime = time.Time{}
	v.speechStartTime = time.Time{}
}

// computeRMS calculates the root-mean-square energy of 16-bit PCM audio (little-endian)
func computeRMS(data []byte) int16 {
	numSamples := len(data) / 2
	if numSamples == 0 {
		return 0
	}

	var sumSquares int64
	for i := 0; i < numSamples; i++ {
		sample := int16(data[i*2]) | int16(data[i*2+1])<<8
		sumSquares += int64(sample) * int64(sample)
	}

	return int16(math.Sqrt(float64(sumSquares) / float64(numSamples)))
}
