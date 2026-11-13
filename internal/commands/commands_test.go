package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type commandSuite struct{ suite.Suite }

func TestCommandSuite(t *testing.T) { suite.Run(t, new(commandSuite)) }

func (s *commandSuite) TestFullRoutes() {
	for _, test := range []struct {
		name      string
		full      FullCommand
		operation string
	}{
		{"toggle", FullCommand{}, "toggle"},
		{"settings", FullCommand{Page: "settings"}, "settings"},
		{"appearance", FullCommand{Page: "settings", Section: "appearance"}, "settings:appearance"},
		{"shortcut", FullCommand{Page: "settings", Section: "shortcut"}, "settings:shortcut"},
	} {
		s.Run(test.name, func() {
			operation, err := Resolve(test.full.Request())
			s.Require().NoError(err)
			s.Equal(test.operation, operation)
		})
	}
}

func (s *commandSuite) TestInvalidFullOptions() {
	for _, args := range [][]string{
		{"--help"},
		{"-h"},
		{"--unknown"},
		{"--page"},
		{"--page", "bogus"},
		{"--section"},
		{"--section", "appearance"},
		{"--page", "settings", "--section", "unknown"},
	} {
		s.Run(strings.Join(args, "_"), func() {
			_, err := Resolve(Request{Kind: "cmd", Name: "full", Args: args})
			s.Error(err)
		})
	}
}

func (s *commandSuite) TestRepeatedSectionUsesLastChoice() {
	operation, err := Resolve(Request{Kind: "cmd", Name: "full", Args: []string{
		"--page", "settings", "--section", "appearance", "--section", "shortcut",
	}})
	s.Require().NoError(err)
	s.Equal("settings:shortcut", operation)
}
