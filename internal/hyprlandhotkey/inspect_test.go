package hyprlandhotkey

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type InspectionSuite struct{ suite.Suite }

func TestInspectionSuite(t *testing.T) { suite.Run(t, new(InspectionSuite)) }

const testDevices = `{"keyboards":[{"rules":"evdev","model":"pc105",` +
	`"layout":"us,ru","variant":",","options":"grp:alt_shift_toggle"}]}`

func (s *InspectionSuite) TestValidation() {
	s.NoError((Combination{Keycode: 46, Modifiers: 4}).Validate())
	s.Error((Combination{Keycode: 8}).Validate())
	s.Error((Combination{Keycode: 46, Modifiers: 2}).Validate())
}

func (s *InspectionSuite) TestMalformedSnapshotsFailClosed() {
	_, err := inspectBindings([]byte("{"), []byte(testDevices), nil, Combination{Keycode: 46})
	s.Error(err)
	_, err = inspectBindings([]byte("[]"), []byte("{"), nil, Combination{Keycode: 46})
	s.Error(err)
	result, err := inspectBindings([]byte(`[{"key":"L"}]`), []byte(testDevices), nil, Combination{Keycode: 46})
	s.NoError(err)
	s.Equal("unknown", result.State)
}

func (s *InspectionSuite) TestPhysicalOverlapIgnoresModifierDifferenceAndUsesGroups() {
	for _, test := range []struct {
		key  string
		code uint32
	}{
		{key: "L", code: 46},
		{key: "Cyrillic_ef", code: 38},
		{key: "exclam", code: 10},
	} {
		s.Run(test.key, func() {
			binds := []byte(`[{"modmask":0,"key":"` + test.key + `","arg":"foreign"}]`)
			result, err := inspectBindings(binds, []byte(testDevices), nil, Combination{
				Keycode: test.code, Modifiers: 64,
			})
			s.Require().NoError(err)
			s.Equal("foreign", result.State)
			s.Len(result.Foreign, 1)
			s.True(result.Potential)
		})
	}
}

func (s *InspectionSuite) TestDevicesAreUnionedAcrossLayouts() {
	devices := []byte(`{"keyboards":[{"layout":"us"},{"layout":"de"}]}`)
	binds := []byte(`[{"modmask":0,"key":"Z"}]`)
	result, err := inspectBindings(binds, devices, nil, Combination{Keycode: 29})
	s.Require().NoError(err)
	s.Equal("foreign", result.State)
}

func (s *InspectionSuite) TestUnknownMappingPreservesUnknownWithKnownConflict() {
	binds := []byte(`[{"modmask":0,"key":"L"},{"modmask":0,"key":"not_a_real_keysym"}]`)
	result, err := inspectBindings(binds, []byte(testDevices), nil, Combination{Keycode: 46})
	s.Require().NoError(err)
	s.Equal("unknown", result.State)
	s.True(result.Potential)
	s.Len(result.Foreign, 1)
}

func (s *InspectionSuite) TestCatchallAndSubmaps() {
	devices := []byte(`{"keyboards":[{"layout":"us"}]}`)
	result, err := inspectBindings([]byte(`[{"modmask":0,"key":"catchall"}]`), devices, nil, Combination{Keycode: 46})
	s.Require().NoError(err)
	s.Equal("foreign", result.State)
	result, err = inspectBindings(
		[]byte(`[{"modmask":0,"submap":"resize","key":"L"}]`), devices, nil, Combination{Keycode: 46},
	)
	s.Require().NoError(err)
	s.Equal("free", result.State)
}

func (s *InspectionSuite) TestOwnedRequiresExactLiveIdentityAndMask() {
	devices := []byte(`{"keyboards":[{"layout":"us"}]}`)
	owned := []Binding{{
		Handle: "trusted", Arg: "runix:toggle", Keycode: 46, Modifiers: 4, Enabled: true, Action: "toggle",
	}}
	foreign := `{"modmask":4,"key":"L","keycode":46,"arg":"runix:toggle","description":"forged"}`
	result, err := inspectBindings([]byte("["+foreign+"]"), devices, owned, Combination{Keycode: 46, Modifiers: 4})
	s.Require().NoError(err)
	s.Equal("owned", result.State)
	s.Equal("trusted", result.Owned[0].Handle)
	result, err = inspectBindings([]byte("["+foreign+"]"), devices, owned, Combination{Keycode: 46, Modifiers: 64})
	s.Require().NoError(err)
	s.Equal("free", result.State, "known owned bindings use exact modifiers, not unknown foreign flags")
	owned[0].Enabled = false
	result, err = inspectBindings([]byte("["+foreign+"]"), devices, owned, Combination{Keycode: 46, Modifiers: 4})
	s.Require().NoError(err)
	s.Equal("free", result.State)
}

func (s *InspectionSuite) TestOwnedAndForeignOverlapConflicts() {
	devices := []byte(`{"keyboards":[{"layout":"us"}]}`)
	owned := []Binding{{
		Handle: "trusted", Arg: "runix:toggle", Keycode: 46, Modifiers: 4, Enabled: true, Action: "toggle",
	}}
	binds := []byte(`[{"modmask":4,"key":"L","keycode":46,"arg":"runix:toggle"},` +
		`{"modmask":0,"key":"L","keycode":46,"arg":"foreign"}]`)
	result, err := inspectBindings(binds, devices, owned, Combination{Keycode: 46, Modifiers: 4})
	s.Require().NoError(err)
	s.Equal("conflicting", result.State)
	s.Len(result.Owned, 1)
	s.Len(result.Foreign, 1)
}

func (s *InspectionSuite) TestForeignOverlapReportsTriggers() {
	devices := []byte(`{"keyboards":[{"layout":"us"}]}`)
	binds := []byte(`[{"modmask":0,"key":"L","keycode":46,"description":"toggle",` +
		`"release":true,"repeat":true,"locked":true,"longPress":true}]`)
	result, err := inspectBindings(binds, devices, nil, Combination{Keycode: 46})
	s.Require().NoError(err)
	s.Equal("foreign", result.State)
	s.True(result.Foreign[0].Release)
	s.True(result.Foreign[0].Repeat)
	s.True(result.Foreign[0].Locked)
	s.True(result.Foreign[0].LongPress)
}

func (s *InspectionSuite) TestInvalidKeymapOrUnavailableKeycodeIsUnknown() {
	devices := []byte(`{"keyboards":[{"layout":"invalid-layout-name"}]}`)
	result, err := inspectBindings([]byte("[]"), devices, nil, Combination{Keycode: 46})
	s.NoError(err)
	s.Equal("unknown", result.State)
	// evdev declares I120 but does not assign symbols to it in the standard US map.
	result, err = inspectBindings(
		[]byte("[]"), []byte(`{"keyboards":[{"layout":"us"}]}`), nil, Combination{Keycode: 120},
	)
	s.NoError(err)
	s.Equal("unknown", result.State)
}
