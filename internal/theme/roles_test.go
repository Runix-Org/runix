package theme

import (
	"math"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RolesSuite struct{ suite.Suite }

func TestRolesSuite(t *testing.T) {
	suite.Run(t, new(RolesSuite))
}

func (s *RolesSuite) fixture() Colors {
	return Colors{
		Background: RGB{12, 14, 18}, BackgroundSecondary: RGB{12, 14, 18},
		Text: RGB{230, 232, 238}, Selection: RGB{42, 130, 220}, Loader: RGB{170, 170, 170},
		Red: RGB{230, 65, 70}, Orange: RGB{230, 120, 30}, Yellow: RGB{230, 200, 40},
		Green: RGB{50, 190, 110}, Blue: RGB{42, 130, 220}, Purple: RGB{150, 90, 200}, Magenta: RGB{210, 70, 150},
	}
}

func (s *RolesSuite) role(roles Roles, name string) Foreground {
	for _, role := range roles.Foregrounds {
		if role.Name == name {
			return role
		}
	}
	s.Require().FailNow("Missing role", name)
	return Foreground{}
}

func (s *RolesSuite) TestRoundedAlphaAndOverlayOrder() {
	white, black := Color{1, 1, 1, 1}, Color{A: 1}
	s.InDelta(26.0/255, white.WithAlpha(Alpha10).Over(black).R, 1e-15)
	s.NotEqual(0.1, white.WithAlpha(Alpha10).Over(black).R)
	s.InDelta(1-(1-Alpha10)*(1-Alpha5),
		white.WithAlpha(Alpha5).Over(white.WithAlpha(Alpha10).Over(black)).R, 1e-15)
	// Contrast must use the rendered overlay, not opaque white or linear-light compositing.
	s.InDelta(1.206597, Contrast(white.WithAlpha(Alpha10), black), 0.00001)
}

func (s *RolesSuite) TestTextSelectionIsNotRowSelection() {
	roles := Derive(s.fixture(), false)
	s.Equal(Alpha10, roles.Surfaces.Selected.A)
	s.Equal(1.0, roles.Surfaces.Selection.A)
	s.Equal(Opaque(s.fixture().Text), s.role(roles, "primary").Effective)
	s.Equal(OnSolid(s.fixture(), roles.Surfaces.Selection), s.role(roles, "selected-text").Effective)
	s.NotEqual(s.role(roles, "primary").Effective, s.role(roles, "selected-text").Effective)
}

func (s *RolesSuite) TestOnSolidUsesLightDarkAndTieOrder() {
	colors := s.fixture()
	colors.Text, colors.Background, colors.BackgroundSecondary = RGB{120, 120, 120}, RGB{}, RGB{255, 255, 255}
	s.Equal(Opaque(colors.BackgroundSecondary), OnSolid(colors, Color{A: 1}))
	s.Equal(Opaque(colors.Background), OnSolid(colors, Color{1, 1, 1, 1}))
	colors.Text = RGB{255, 255, 255}
	s.Equal(Opaque(colors.Text), OnSolid(colors, Color{A: 1}))
}

func (s *RolesSuite) TestGradientSamplesIncludeIntermediateAndExactStop() {
	colors := s.fixture()
	colors.Background, colors.BackgroundSecondary = RGB{}, RGB{255, 255, 255}
	roles := Derive(colors, false)
	s.Equal(Color{0.5, 0.5, 0.5, 1}, roles.Surfaces.At(0.35))
	s.Equal(Opaque(colors.BackgroundSecondary), roles.Surfaces.At(0.7))
	s.Equal(roles.Surfaces.At(0.7), roles.Surfaces.At(1))
	samples := neutralSamples(roles.Surfaces)
	var stop, intermediate bool
	for _, sample := range samples {
		stop = stop || sample.Y == 0.7
		intermediate = intermediate || sample.Y == 0.5
	}
	s.True(stop)
	s.True(intermediate)
	primary := s.role(roles, "primary")
	s.Less(primary.Diagnostic.Minimum, Contrast(primary.Effective, roles.Surfaces.B0))
	s.Less(primary.Diagnostic.Minimum, Contrast(primary.Effective, roles.Surfaces.B1))
	s.Greater(primary.Diagnostic.Worst.Y, 0.0)
	s.Less(primary.Diagnostic.Worst.Y, 0.7)
}

func (s *RolesSuite) TestCorrectionPreservesSourceFillsAndPassingForegrounds() {
	colors := s.fixture()
	normal, corrected := Derive(colors, false), Derive(colors, true)
	s.Equal(colors, corrected.Original)
	s.Equal(normal.Surfaces, corrected.Surfaces)
	s.Equal(normal, Derive(colors, false))
	s.Equal(corrected, Derive(colors, true))
	for _, role := range normal.Foregrounds {
		effective := s.role(corrected, role.Name)
		if role.Diagnostic.Passing {
			s.Equal(role.Effective, effective.Effective, role.Name)
		}
		s.Equal(role.Original, effective.Original, role.Name)
		s.InDelta(effective.Diagnostic.Minimum,
			Contrast(effective.Effective, effective.Diagnostic.Worst.Color), 1e-12, role.Name)
	}
	secondary := s.role(corrected, "secondary")
	s.True(secondary.Diagnostic.Passing)
	s.Equal(s.role(normal, "disabled").Effective, s.role(corrected, "disabled").Effective)
}

func (s *RolesSuite) TestEqualColorsNeedRGBChangeWithoutSurfaceChange() {
	colors := s.fixture()
	colors.Text, colors.Selection = colors.Background, colors.Background
	roles := Derive(colors, true)
	for _, name := range []string{"primary", "placeholder", "focus", "selection-marker", "selected-text"} {
		role := s.role(roles, name)
		s.True(role.Diagnostic.Passing, name)
		s.True(role.Diagnostic.Changed, name)
		s.GreaterOrEqual(role.Diagnostic.Minimum+RatioTolerance, role.Diagnostic.Target, name)
	}
	s.Equal(Derive(colors, false).Surfaces, roles.Surfaces)
}

func (s *RolesSuite) TestImpossibleGradientReportsFailureAndBestSampledCandidate() {
	colors := s.fixture()
	colors.Background, colors.BackgroundSecondary = RGB{}, RGB{255, 255, 255}
	normal, corrected := Derive(colors, false), Derive(colors, true)
	s.Equal(normal.Surfaces, corrected.Surfaces)
	primary := s.role(corrected, "primary")
	s.False(primary.Diagnostic.Passing)
	s.Less(primary.Diagnostic.Minimum, 4.5)
	s.GreaterOrEqual(primary.Diagnostic.Minimum+contrastTie, s.role(normal, "primary").Diagnostic.Minimum)
	evaluator := newEvaluator(neutralSamples(corrected.Surfaces))
	for _, endpoint := range [...]Color{{A: 1}, {1, 1, 1, 1}} {
		for index := range fallbackSteps + 1 {
			candidate := interpolate(primary.Original.WithAlpha(1), endpoint, float64(index)/fallbackSteps)
			minimum, _ := evaluator.minimum(candidate)
			s.GreaterOrEqual(primary.Diagnostic.Minimum+contrastTie, minimum)
		}
	}
}

func (s *RolesSuite) TestCorrectionFindsNarrowFeasibleInterval() {
	// Two backgrounds leave a tiny passing luminance gap around 0.18.
	value, target := 0.18, 4.5
	dark := (value+0.05)/target - 0.05
	light := target*(value+0.05) - 0.05
	encoded := func(value float64) float64 {
		if value <= 0.0031308 {
			return 12.92 * value
		}
		return 1.055*math.Pow(value, 1/2.4) - 0.055
	}
	a, b := encoded(dark-1e-7), encoded(light+1e-7)
	evaluator := newEvaluator([]BackgroundSample{
		{Root, 0, Color{a, a, a, 1}}, {Popup, -1, Color{b, b, b, 1}},
	})
	original := Color{0.7, 0.7, 0.7, 1}
	corrected := evaluator.correct(original, target)
	minimum, _ := evaluator.minimum(corrected)
	s.GreaterOrEqual(minimum+RatioTolerance, target)
	s.InDelta(value, luminance(corrected), 1e-6)
}

func (s *RolesSuite) TestOpacitySolverAgainstFinalCompositing() {
	evaluator := newEvaluator([]BackgroundSample{
		{Root, 0, Color{0.02, 0.04, 0.08, 1}},
		{Popup, -1, Color{0.08, 0.04, 0.02, 1}},
	})
	original := Color{0.9, 0.8, 0.7, Alpha40}
	corrected := evaluator.correct(original, 4.5)
	s.Equal(original.R, corrected.R)
	s.Greater(corrected.A, original.A)
	minimum, _ := evaluator.minimum(corrected)
	s.InDelta(4.5, minimum, 1e-8)
	previous := corrected.WithAlpha(corrected.A - 1e-7)
	before, _ := evaluator.minimum(previous)
	s.Less(before, 4.5)
}
