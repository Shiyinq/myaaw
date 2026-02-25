package voice

import (
	"log"
	"sync"

	"github.com/gordonklaus/portaudio"
)

const (
	// Input audio config (to Gemini)
	InputSampleRate = 16000
	InputChannels   = 1
	InputFrameSize  = 1024 // samples per buffer

	// Output audio config (from Gemini)
	OutputSampleRate = 24000
	OutputChannels   = 1
	OutputFrameSize  = 512 // smaller frames for lower latency
)

// AudioCapture captures microphone input as 16-bit PCM, 16kHz, mono
type AudioCapture struct {
	stream *portaudio.Stream
	buffer []int16
}

// NewAudioCapture creates a new microphone capture instance
func NewAudioCapture() (*AudioCapture, error) {
	buffer := make([]int16, InputFrameSize)
	stream, err := portaudio.OpenDefaultStream(
		InputChannels, // input channels
		0,             // output channels
		float64(InputSampleRate),
		InputFrameSize,
		buffer,
	)
	if err != nil {
		return nil, err
	}

	return &AudioCapture{
		stream: stream,
		buffer: buffer,
	}, nil
}

// Start begins capturing audio
func (a *AudioCapture) Start() error {
	return a.stream.Start()
}

// Stop pauses audio capture without closing the stream
func (a *AudioCapture) Stop() error {
	return a.stream.Stop()
}

// Read reads the next chunk of audio data as raw bytes (little-endian int16)
func (a *AudioCapture) Read() ([]byte, error) {
	err := a.stream.Read()
	if err != nil {
		return nil, err
	}

	// Convert int16 slice to bytes (little-endian)
	bytes := make([]byte, len(a.buffer)*2)
	for i, sample := range a.buffer {
		bytes[i*2] = byte(sample)
		bytes[i*2+1] = byte(sample >> 8)
	}
	return bytes, nil
}

// Close stops and closes the audio stream
func (a *AudioCapture) Close() {
	if a.stream != nil {
		if err := a.stream.Stop(); err != nil {
			log.Printf("Error stopping audio capture: %v", err)
		}
		if err := a.stream.Close(); err != nil {
			log.Printf("Error closing audio capture: %v", err)
		}
	}
}

// AudioPlayer plays PCM audio using a simple queued approach.
// Audio samples are accumulated in an internal buffer, and a dedicated
// goroutine continuously drains it to PortAudio in fixed-size writes.
type AudioPlayer struct {
	stream *portaudio.Stream
	outBuf []int16 // PortAudio write buffer

	mu       sync.Mutex
	sampleQ  []int16 // accumulated samples waiting to be played
	playing  bool
	stopChan chan struct{}
	doneChan chan struct{} // signals drainLoop has exited
}

// NewAudioPlayer creates a new audio output instance.
// sampleRate defaults to OutputSampleRate (24000) if 0 is passed.
func NewAudioPlayer(sampleRate int) (*AudioPlayer, error) {
	if sampleRate == 0 {
		sampleRate = OutputSampleRate
	}
	outBuf := make([]int16, OutputFrameSize)
	stream, err := portaudio.OpenDefaultStream(
		0,              // input channels
		OutputChannels, // output channels
		float64(sampleRate),
		OutputFrameSize,
		&outBuf,
	)
	if err != nil {
		return nil, err
	}

	return &AudioPlayer{
		stream:   stream,
		outBuf:   outBuf,
		sampleQ:  make([]int16, 0, sampleRate*2), // pre-alloc 2s
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}, nil
}

// Start begins the audio output stream and playback goroutine
func (p *AudioPlayer) Start() error {
	if err := p.stream.Start(); err != nil {
		return err
	}

	// Start the drain goroutine
	go p.drainLoop()
	return nil
}

// drainLoop continuously writes queued samples to PortAudio
func (p *AudioPlayer) drainLoop() {
	defer close(p.doneChan)
	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		p.mu.Lock()
		available := len(p.sampleQ)

		if available >= OutputFrameSize {
			// Copy a full frame from queue to the output buffer
			copy(p.outBuf, p.sampleQ[:OutputFrameSize])
			p.sampleQ = p.sampleQ[OutputFrameSize:]
			p.playing = true
			p.mu.Unlock()

			// Write the buffer (this blocks until PortAudio consumes it)
			if err := p.stream.Write(); err != nil {
				select {
				case <-p.stopChan:
					return // shutting down, ignore error
				default:
					log.Printf("Audio write error: %v", err)
				}
			}
		} else if available > 0 {
			// Write a partial frame: copy what we have, zero-pad the rest
			copy(p.outBuf, p.sampleQ)
			for i := available; i < OutputFrameSize; i++ {
				p.outBuf[i] = 0
			}
			p.sampleQ = p.sampleQ[:0]
			p.playing = true
			p.mu.Unlock()

			if err := p.stream.Write(); err != nil {
				select {
				case <-p.stopChan:
					return
				default:
					log.Printf("Audio write error: %v", err)
				}
			}
		} else {
			// Nothing to play — write silence to keep the stream alive
			p.playing = false
			p.mu.Unlock()

			for i := range p.outBuf {
				p.outBuf[i] = 0
			}
			if err := p.stream.Write(); err != nil {
				select {
				case <-p.stopChan:
					return
				default:
					log.Printf("Audio write error: %v", err)
				}
			}
		}
	}
}

// Play enqueues raw PCM bytes (little-endian int16) for playback
func (p *AudioPlayer) Play(data []byte) error {
	numSamples := len(data) / 2
	samples := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}

	p.mu.Lock()
	p.sampleQ = append(p.sampleQ, samples...)
	p.mu.Unlock()

	return nil
}

// Flush clears the current audio playback queue immediately
func (p *AudioPlayer) Flush() {
	p.mu.Lock()
	p.sampleQ = p.sampleQ[:0]
	p.playing = false
	p.mu.Unlock()
}

// IsPlaying returns true if there is audio data being played
func (p *AudioPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing || len(p.sampleQ) > 0
}

// Close stops and closes the audio stream
func (p *AudioPlayer) Close() {
	close(p.stopChan)
	<-p.doneChan // wait for drainLoop to exit
	if p.stream != nil {
		if err := p.stream.Stop(); err != nil {
			log.Printf("Error stopping audio player: %v", err)
		}
		if err := p.stream.Close(); err != nil {
			log.Printf("Error closing audio player: %v", err)
		}
	}
}
