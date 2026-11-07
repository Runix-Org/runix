package theme

import "math"

// Context identifies the actual surface beneath a foreground, including wash order.
type Context string

const (
	Root         Context = "root"
	Popup        Context = "popup"
	SelectedText Context = "selected-text"
	Checked      Context = "checked"
	CheckedHover Context = "checked-hover"
	CheckedPress Context = "checked-press"
)

// BackgroundSample records the final opaque surface. Y is root-normalized, or -1 for solid fills.
type BackgroundSample struct {
	Context Context
	Y       float64
	Color   Color
}

// Diagnostic is a sampled minimum, not a continuous-gradient conformance claim.
// Target zero denotes a decorative or disabled role excluded from pass/fail gates.
type Diagnostic struct {
	Minimum float64
	Target  float64
	Worst   BackgroundSample
	Passing bool
	Changed bool
}

// Foreground separates source derivation from the effective, optionally corrected color.
// Contexts describes all backgrounds used by the diagnostic, including interaction states.
type Foreground struct {
	Name       string
	Original   Color
	Effective  Color
	Contexts   []Context
	Diagnostic Diagnostic
}

// Surfaces are immutable under correction. Washes remain explicit overlays.
type Surfaces struct {
	B0, B1                                      Color
	Selected, Hover, Press, Field, Button       Color
	Separator, Border, HoverBorder              Color
	DisabledBorder, DisabledTrack, Track        Color
	Scrollbar, ScrollbarHover, ScrollbarPressed Color
	Scrim, Selection, Checked                   Color
	Tags                                        [7]Color
}

// At returns the one root gradient: B0 at 0%, B1 at 70%, then constant B1.
func (surfaces Surfaces) At(y float64) Color {
	return interpolate(surfaces.B0, surfaces.B1, math.Max(0, math.Min(y/0.7, 1)))
}

// Roles is one coherent snapshot, calculated only when theme or correction changes.
// Semantic hue order is red/error, orange, yellow/warning, green/success, blue/link/info,
// purple, magenta. Branded assets are deliberately outside this contract.
type Roles struct {
	Original        Colors
	ImproveContrast bool
	Surfaces        Surfaces
	Foregrounds     []Foreground
}

var hueNames = [...]string{"red", "orange", "yellow", "green", "blue", "purple", "magenta"}

// Derive retains every original color and all fills, even when contrast is impossible.
func Derive(colors Colors, improveContrast bool) Roles {
	t, selection := Opaque(colors.Text), Opaque(colors.Selection)
	surfaces := Surfaces{
		B0: Opaque(colors.Background), B1: Opaque(colors.BackgroundSecondary),
		Selected: selection.WithAlpha(Alpha10), Hover: t.WithAlpha(Alpha5), Press: t.WithAlpha(Alpha10),
		Field: t.WithAlpha(Alpha5), Button: t.WithAlpha(Alpha10),
		Separator: t.WithAlpha(Alpha10), Border: t.WithAlpha(Alpha20), HoverBorder: t.WithAlpha(Alpha40),
		DisabledBorder: t.WithAlpha(Alpha10), DisabledTrack: t.WithAlpha(Alpha10), Track: t.WithAlpha(Alpha20),
		Scrollbar: t.WithAlpha(Alpha20), ScrollbarHover: t.WithAlpha(Alpha40),
		ScrollbarPressed: t.WithAlpha(Alpha60), Scrim: Color{A: Alpha40},
		Selection: selection, Checked: selection,
	}
	hues := [...]RGB{colors.Red, colors.Orange, colors.Yellow, colors.Green, colors.Blue, colors.Purple, colors.Magenta}
	for index, hue := range hues {
		surfaces.Tags[index] = Opaque(hue).WithAlpha(Alpha15)
	}
	roles := Roles{
		Original: colors, ImproveContrast: improveContrast, Surfaces: surfaces,
		Foregrounds: make([]Foreground, 0, 32),
	}
	neutral := neutralSamples(surfaces)
	neutralEvaluator := newEvaluator(neutral)
	add := func(name string, original Color, target float64, evaluator contrastEvaluator) {
		roles.Foregrounds = append(roles.Foregrounds,
			deriveForeground(name, original, target, evaluator, improveContrast))
	}
	add("primary", t, 4.5, neutralEvaluator)
	add("secondary", t.WithAlpha(Alpha60), 4.5, neutralEvaluator)
	placeholderSamples := make([]BackgroundSample, 0, len(neutral))
	for _, sample := range neutral {
		if sample.Context == Root || sample.Context == Popup ||
			sample.Context == "root/field" || sample.Context == "popup/field" {
			placeholderSamples = append(placeholderSamples, sample)
		}
	}
	add("placeholder", t.WithAlpha(Alpha40), 4.5, newEvaluator(placeholderSamples))
	add("disabled", t.WithAlpha(Alpha40), 0, neutralEvaluator)
	add("focus", selection, 3, neutralEvaluator)
	add("selection-marker", selection, 3, neutralEvaluator)
	add("caret", t, 3, neutralEvaluator)
	add("monochrome-icon", t, 3, neutralEvaluator)
	add("secondary-icon", t.WithAlpha(Alpha60), 3, neutralEvaluator)
	add("loader", Opaque(colors.Loader), 3, neutralEvaluator)
	add("selected-text", OnSolid(colors, selection), 4.5,
		newEvaluator([]BackgroundSample{{SelectedText, -1, selection}}))
	checked := []BackgroundSample{
		{Checked, -1, selection},
		{CheckedHover, -1, surfaces.Hover.Over(selection)},
		{CheckedPress, -1, surfaces.Press.Over(selection)},
	}
	add("on-checked", OnSolid(colors, selection), 3, newEvaluator(checked))
	// Slider fill and thumb are meaningful indicators; their adjacent track is not a flat root.
	indicatorSamples := make([]BackgroundSample, 0, len(neutral)*2)
	indicatorSamples = append(indicatorSamples, neutral...)
	var previous, trackContext Context
	for _, sample := range neutral {
		if sample.Context != previous {
			previous = sample.Context
			trackContext = Context(string(sample.Context) + "/track")
		}
		indicatorSamples = append(indicatorSamples, BackgroundSample{
			trackContext, sample.Y, surfaces.Track.Over(sample.Color),
		})
	}
	indicatorEvaluator := newEvaluator(indicatorSamples)
	add("slider-fill", selection, 3, indicatorEvaluator)
	add("slider-thumb", t, 3, indicatorEvaluator)
	add("switch-thumb", t, 3, indicatorEvaluator)
	add("unchecked-outline", t.WithAlpha(Alpha60), 3, neutralEvaluator)
	for index, hue := range hues {
		color := Opaque(hue)
		tagSamples := make([]BackgroundSample, 0, len(neutral)*3)
		for _, state := range [...]struct {
			name string
			wash Color
		}{{"tag", Color{}}, {"tag-hover", surfaces.Hover}, {"tag-press", surfaces.Press}} {
			var last, tagContext Context
			for _, sample := range neutral {
				if sample.Context != Root && sample.Context != Popup &&
					sample.Context != "root/selected" && sample.Context != "popup/selected" {
					continue
				}
				fill := surfaces.Tags[index].Over(sample.Color)
				if sample.Context != last {
					last = sample.Context
					tagContext = Context(string(sample.Context) + "/" + hueNames[index] + "/" + state.name)
				}
				tagSamples = append(tagSamples, BackgroundSample{
					tagContext,
					sample.Y, state.wash.Over(fill),
				})
			}
		}
		// Semantic text can appear directly or in its own tag; icons use a separate target.
		tagEvaluator := newEvaluator(append(tagSamples, neutral...))
		add(hueNames[index]+"-text", color, 4.5, tagEvaluator)
		add(hueNames[index]+"-icon", color, 3, tagEvaluator)
	}
	return roles
}

func neutralSamples(surfaces Surfaces) []BackgroundSample {
	// Include the exact gradient stop in addition to 257 uniform positions.
	positions := make([]float64, 0, 258)
	for index := range 257 {
		positions = append(positions, float64(index)/256)
	}
	positions = append(positions, 0.7)
	states := [...]struct {
		name string
		fill Color
		wash Color
	}{
		{"", Color{}, Color{}},
		{"/hover", Color{}, surfaces.Hover},
		{"/press", Color{}, surfaces.Press},
		{"/selected", surfaces.Selected, Color{}},
		{"/field", surfaces.Field, Color{}},
		{"/button", surfaces.Button, Color{}},
		{"/button-hover", surfaces.Button, surfaces.Hover},
		{"/button-press", surfaces.Button, surfaces.Press},
	}
	samples := make([]BackgroundSample, 0, (len(positions)+1)*len(states))
	for _, base := range [...]Context{Root, Popup} {
		count := len(positions)
		if base == Popup {
			count = 1
		}
		for _, state := range states {
			context := Context(string(base) + state.name)
			for index := range count {
				y, background := -1.0, surfaces.B1
				if base == Root {
					y = positions[index]
					background = surfaces.At(y)
				}
				samples = append(samples, BackgroundSample{
					context, y, state.wash.Over(state.fill.Over(background)),
				})
			}
		}
	}
	return samples
}
