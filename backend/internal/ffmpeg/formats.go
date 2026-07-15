package ffmpeg

// Resolution defines output dimensions.
type Resolution struct {
	Width  int
	Height int
	Label  string
}

var (
	Resolution720p  = Resolution{Width: 1280, Height: 720, Label: "720p"}
	Resolution1080p = Resolution{Width: 1920, Height: 1080, Label: "1080p"}
	Resolution4K    = Resolution{Width: 3840, Height: 2160, Label: "4K"}
)

// Maps resolution label strings to Resolution structs.
var Resolutions = map[string]Resolution{
	"720p":  Resolution720p,
	"1080p": Resolution1080p,
	"4k":    Resolution4K,
	"4K":    Resolution4K,
}

// Default resolution when none specified.
var DefaultResolution = Resolution1080p
