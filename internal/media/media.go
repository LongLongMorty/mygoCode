package media

import "context"

// ImageRequest holds parameters for image generation.
type ImageRequest struct {
	Prompt string
	Size   string
}

// ImageResult holds a single generated image.
type ImageResult struct {
	Bytes []byte
	Ext   string
}

// VideoRequest holds parameters for video generation.
type VideoRequest struct {
	Prompt      string
	InputImages []string
	NumFrames   int
}

// VideoResult holds a generated video.
type VideoResult struct {
	Bytes []byte
	Ext   string
}

// ProgressFn is called with progress updates during video generation.
type ProgressFn func(pct float64)

// ImageProvider generates images.
type ImageProvider interface {
	Name() string
	GenerateImage(context.Context, ImageRequest) ([]ImageResult, error)
}

// VideoProvider generates videos.
type VideoProvider interface {
	Name() string
	GenerateVideo(context.Context, VideoRequest, ProgressFn) (*VideoResult, error)
}
