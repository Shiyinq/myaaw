package voice

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/svanichkin/gocam"
)

const (
	// Default JPEG quality for video frames
	JPEGQuality = 50
	// Max width for captured frames (height scales proportionally)
	MaxFrameWidth = 1280
)

// VideoMode represents the source of video frames
type VideoMode int

const (
	VideoModeNone   VideoMode = iota
	VideoModeScreen           // capture screen
	VideoModeCamera           // capture webcam
)

// VideoCapturer periodically captures frames from screen or camera
type VideoCapturer struct {
	modes    []VideoMode
	interval time.Duration

	// Camera stream state
	camMu        sync.Mutex
	camStream    <-chan gocam.Frame
	camCancel    context.CancelFunc
	camLastFrame *gocam.Frame
}

// NewVideoCapturer creates a new video capturer with specified modes and interval
func NewVideoCapturer(modes []VideoMode, interval time.Duration) *VideoCapturer {
	return &VideoCapturer{
		modes:    modes,
		interval: interval,
	}
}

// Run starts capturing frames and sends JPEG data to the provided channel.
// This function blocks and should be run in a goroutine.
func (v *VideoCapturer) Run(ctx context.Context, frameChan chan<- []byte) {
	// Start persistent camera stream if camera mode is enabled
	for _, mode := range v.modes {
		if mode == VideoModeCamera {
			if err := v.startCameraStream(ctx); err != nil {
				log.Printf("Warning: could not start camera stream: %v", err)
			}
			break
		}
	}

	// Start a goroutine to continuously consume camera frames (keep latest)
	if v.camStream != nil {
		go v.consumeCameraFrames()
	}

	ticker := time.NewTicker(v.interval)
	defer ticker.Stop()
	defer v.stopCameraStream()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, mode := range v.modes {
				var frame []byte
				var err error

				switch mode {
				case VideoModeScreen:
					frame, err = captureScreen()
				case VideoModeCamera:
					frame, err = v.captureCamera()
				}

				if err != nil {
					log.Printf("Video capture error (%d): %v", mode, err)
					continue
				}

				if frame != nil {
					select {
					case frameChan <- frame:
					default:
						// drop frame if channel is full
					}
				}
			}
		}
	}
}

// startCameraStream initializes a persistent camera stream
func (v *VideoCapturer) startCameraStream(parentCtx context.Context) error {
	camCtx, cancel := context.WithCancel(parentCtx)
	stream, err := gocam.StartStream(camCtx)
	if err != nil {
		cancel()
		return err
	}

	v.camStream = stream
	v.camCancel = cancel
	log.Println("Camera stream started (persistent)")
	return nil
}

// consumeCameraFrames runs in a goroutine and keeps the latest camera frame
func (v *VideoCapturer) consumeCameraFrames() {
	for frame := range v.camStream {
		v.camMu.Lock()
		frameCopy := gocam.Frame{
			Data:   make([]byte, len(frame.Data)),
			Width:  frame.Width,
			Height: frame.Height,
		}
		copy(frameCopy.Data, frame.Data)
		v.camLastFrame = &frameCopy
		v.camMu.Unlock()
	}
}

// stopCameraStream cleans up the camera stream
func (v *VideoCapturer) stopCameraStream() {
	if v.camCancel != nil {
		v.camCancel()
	}
}

// captureCamera grabs the latest frame from the persistent camera stream
func (v *VideoCapturer) captureCamera() ([]byte, error) {
	v.camMu.Lock()
	frame := v.camLastFrame
	v.camMu.Unlock()

	if frame == nil {
		return nil, nil // no frame yet
	}

	if frame.Width <= 0 || frame.Height <= 0 || len(frame.Data) != frame.Width*frame.Height*3 {
		return nil, nil
	}

	// Convert YCbCr 4:4:4 packed bytes to image.NRGBA
	img := image.NewNRGBA(image.Rect(0, 0, frame.Width, frame.Height))
	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			idx := (y*frame.Width + x) * 3
			yVal := frame.Data[idx]
			cb := frame.Data[idx+1]
			cr := frame.Data[idx+2]
			r, g, b := color.YCbCrToRGB(yVal, cb, cr)
			di := img.PixOffset(x, y)
			img.Pix[di+0] = r
			img.Pix[di+1] = g
			img.Pix[di+2] = b
			img.Pix[di+3] = 0xff
		}
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JPEGQuality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// captureScreen takes a screenshot of the primary display
func captureScreen() ([]byte, error) {
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}

	return resizeAndEncodeJPEG(img)
}

// resizeAndEncodeJPEG resizes the image if needed and encodes as JPEG
func resizeAndEncodeJPEG(img image.Image) ([]byte, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Simple downscale if too large: subsample the image
	var finalImg image.Image = img
	if width > MaxFrameWidth {
		ratio := float64(MaxFrameWidth) / float64(width)
		newWidth := MaxFrameWidth
		newHeight := int(float64(height) * ratio)

		resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		for y := 0; y < newHeight; y++ {
			srcY := int(float64(y) / ratio)
			for x := 0; x < newWidth; x++ {
				srcX := int(float64(x) / ratio)
				resized.Set(x, y, img.At(srcX, srcY))
			}
		}
		finalImg = resized
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, finalImg, &jpeg.Options{Quality: JPEGQuality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
