package session

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

// Limits apply before JPEG pixel allocation. They bound one decoded image,
// not total session memory: composition and callbacks can hold several copies.
const maxScreenDimension = 8192
const maxScreenPixels = 16 * 1024 * 1024

func screenDimensionsAllowed(width, height int) bool {
	return width > 0 && height > 0 && width <= maxScreenDimension && height <= maxScreenDimension && width <= maxScreenPixels/height
}

func screenJPEGConfig(data []byte) (image.Config, error) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Config{}, err
	}
	if !screenDimensionsAllowed(config.Width, config.Height) {
		return image.Config{}, fmt.Errorf("session: JPEG dimensions exceed screen policy")
	}
	return config, nil
}

func decodeScreenJPEG(data []byte) (image.Image, error) {
	if _, err := screenJPEGConfig(data); err != nil {
		return nil, err
	}
	return jpeg.Decode(bytes.NewReader(data))
}
