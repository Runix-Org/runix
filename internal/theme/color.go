package theme

import "math"

// Color is explicit, unpremultiplied encoded-sRGB RGBA in [0,1].
// Keep floating-point channels until the renderer boundary; Qt and CSS hex orders differ.
type Color struct {
	R, G, B, A float64
}

const (
	Alpha5  = 13.0 / 255
	Alpha10 = 26.0 / 255
	Alpha15 = 38.0 / 255
	Alpha20 = 51.0 / 255
	Alpha40 = 102.0 / 255
	Alpha60 = 153.0 / 255
)

// Opaque converts an original RGB byte color without changing its channels.
func Opaque(c RGB) Color {
	return Color{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, 1}
}

// WithAlpha returns an overlay, not a preblended surface.
func (c Color) WithAlpha(alpha float64) Color {
	c.A = alpha
	return c
}

// Over composites onto an opaque background in encoded sRGB, not linear light.
func (c Color) Over(background Color) Color {
	return Color{
		c.A*c.R + (1-c.A)*background.R,
		c.A*c.G + (1-c.A)*background.G,
		c.A*c.B + (1-c.A)*background.B,
		1,
	}
}

func interpolate(a, b Color, amount float64) Color {
	return Color{
		a.R + (b.R-a.R)*amount,
		a.G + (b.G-a.G)*amount,
		a.B + (b.B-a.B)*amount,
		a.A + (b.A-a.A)*amount,
	}
}

func linear(channel float64) float64 {
	if channel <= 0.04045 {
		return channel / 12.92
	}
	return math.Pow((channel+0.055)/1.055, 2.4)
}

func luminance(c Color) float64 {
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

func contrastLuminance(a, b float64) float64 {
	return (math.Max(a, b) + 0.05) / (math.Min(a, b) + 0.05)
}

// Contrast measures the final composited foreground against an opaque background.
func Contrast(foreground, background Color) float64 {
	return contrastLuminance(luminance(foreground.Over(background)), luminance(background))
}

// OnSolid chooses T, B0, B1 by highest contrast, retaining that order on ties.
func OnSolid(colors Colors, fill Color) Color {
	best := Opaque(colors.Text)
	for _, candidate := range [...]RGB{colors.Background, colors.BackgroundSecondary} {
		color := Opaque(candidate)
		if Contrast(color, fill) > Contrast(best, fill) {
			best = color
		}
	}
	return best
}
