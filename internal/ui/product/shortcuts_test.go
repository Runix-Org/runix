package product

import (
	"testing"

	qt "github.com/mappu/miqt/qt6"
	"github.com/stretchr/testify/suite"
)

type recorderSuite struct{ suite.Suite }

func TestRecorderSuite(t *testing.T) { suite.Run(t, new(recorderSuite)) }

func (s *recorderSuite) TestLayoutSymbolsKeepPhysicalIdentity() {
	modifiers := qt.ControlModifier | qt.AltModifier | qt.ShiftModifier | qt.MetaModifier
	latin, message := recordedCombination(38, qt.Key_A, modifiers)
	s.Empty(message)
	cyrillic, message := recordedCombination(38, qt.Key(0x0424), modifiers)
	s.Empty(message)
	s.Equal(latin, cyrillic)
	s.Equal(uint32(38), latin.Keycode)
	s.Equal(uint32(77), latin.Modifiers)
}

func (s *recorderSuite) TestModifierOnlyUnsupportedAndMissingNativeIdentity() {
	for _, key := range []qt.Key{qt.Key_Control, qt.Key_Shift, qt.Key_AltGr, qt.Key_Meta, qt.Key_CapsLock} {
		_, message := recordedCombination(37, key, qt.ControlModifier)
		s.Contains(message, "non-modifier")
	}
	_, message := recordedCombination(38, qt.Key_A, qt.GroupSwitchModifier)
	s.Contains(message, "not supported")
	for _, scan := range []uint{0, 8, 256, 1 << 32} {
		_, message = recordedCombination(scan, qt.Key_A, qt.NoModifier)
		s.Contains(message, "physical XKB code")
	}
	keypad, message := recordedCombination(87, qt.Key_1, qt.KeypadModifier)
	s.Empty(message)
	s.Equal(uint32(87), keypad.Keycode)
	s.Zero(keypad.Modifiers)
}
