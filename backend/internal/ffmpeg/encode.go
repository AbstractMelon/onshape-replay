// Package ffmpeg wraps FFmpeg subprocess invocation.
// This package knows nothing about Onshape or the job lifecycle.
package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Encoder holds the path to the ffmpeg binary.
type Encoder struct {
	BinPath string
}

// Creates an Encoder using the given binary path.
// Pass "ffmpeg" to use PATH lookup.
func NewEncoder(binPath string) *Encoder {
	return &Encoder{BinPath: binPath}
}

// Verifies that the ffmpeg binary is reachable.
func (e *Encoder) CheckAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.BinPath, "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not available at %q: %w", e.BinPath, err)
	}
	return nil
}

// Encodes a PNG frame sequence to an H.264 MP4.
// must be an ffmpeg-compatible frame pattern, e.g. "frames/frame_%04d.png".
func (e *Encoder) EncodeMP4(ctx context.Context, frameGlob, outputPath string, fps int, res Resolution) error {
	// Libx264 with yuv420p for maximum compatibility.
	// Scale filter ensures the output matches the requested resolution.
	args := []string{
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", frameGlob,
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos,format=yuv420p", res.Width, res.Height),
		"-c:v", "libx264",
		"-preset", "slow",
		"-crf", "18",
		"-movflags", "+faststart",
		outputPath,
	}

	return e.run(ctx, args)
}

// Encodes a PNG frame sequence to an animated GIF.
// GIF supports transparency but not true-color; the palette filter improves quality.
func (e *Encoder) EncodeGIF(ctx context.Context, frameGlob, outputPath string, fps int, res Resolution) error {
	// Two-pass GIF: generate palette then apply it.
	palettePath := outputPath + ".palette.png"

	// Pass 1: generate palette.
	pass1 := []string{
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", frameGlob,
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos,palettegen=stats_mode=diff", res.Width, res.Height),
		palettePath,
	}
	if err := e.run(ctx, pass1); err != nil {
		return fmt.Errorf("GIF palette pass: %w", err)
	}

	// Pass 2: encode with palette.
	pass2 := []string{
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", frameGlob,
		"-i", palettePath,
		"-lavfi", fmt.Sprintf("scale=%d:%d:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5", res.Width, res.Height),
		outputPath,
	}
	if err := e.run(ctx, pass2); err != nil {
		return fmt.Errorf("GIF encode pass: %w", err)
	}

	return nil
}

// Executes ffmpeg with the given arguments and captures stderr for errors.
func (e *Encoder) run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, e.BinPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg error: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}
