package desktop

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	FILE0 = "/home/user/file0"
	FILE1 = "/home/user/file1"
	URL0  = "http://example0.com"
	URL1  = "http://example1.com"
)

type DesktopEntryLauncherSuite struct {
	suite.Suite

	urls  []string
	files []string
}

func (s *DesktopEntryLauncherSuite) SetupTest() {
	s.urls = []string{}
	s.files = []string{}
}

func (s *DesktopEntryLauncherSuite) checkArgs(execStr string, expected []string) {
	t := s.T()

	launcher := NewDesktopEntryLauncher("")
	actual, err := launcher.buildLaunchArgs(execStr, s.urls, s.files)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func (s *DesktopEntryLauncherSuite) TestExecStr() {
	s.checkArgs(`vim`, []string{"vim"})
	s.checkArgs(`vim test`, []string{"vim", "test"})

	s.files = []string{FILE0}
	s.checkArgs(`vim`, []string{"vim"})
	s.checkArgs(`vim test`, []string{"vim", "test"})

	s.files = []string{FILE0, FILE1}
	s.checkArgs(`vim`, []string{"vim"})
	s.checkArgs(`vim test`, []string{"vim", "test"})

	s.files = []string{}
	s.urls = []string{URL0}
	s.checkArgs(`vim`, []string{"vim"})
	s.checkArgs(`vim test`, []string{"vim", "test"})

	s.urls = []string{URL0, URL1}
	s.checkArgs(`vim`, []string{"vim"})
	s.checkArgs(`vim test`, []string{"vim", "test"})
}

func (s *DesktopEntryLauncherSuite) TestExecStrWithFileParam() {
	s.checkArgs(`vim %f`, []string{"vim"})
	s.checkArgs(`vim test %f`, []string{"vim", "test"})

	s.files = []string{FILE0}
	s.checkArgs(`vim %f`, []string{"vim", FILE0})
	s.checkArgs(`vim test %f`, []string{"vim", "test", FILE0})

	s.files = []string{FILE0, FILE1}
	s.checkArgs(`vim %f`, []string{"vim", FILE0})
	s.checkArgs(`vim test %f`, []string{"vim", "test", FILE0})

	s.files = []string{}
	s.urls = []string{URL0}
	s.checkArgs(`vim %f`, []string{"vim"})
	s.checkArgs(`vim test %f`, []string{"vim", "test"})

	s.urls = []string{URL0, URL1}
	s.checkArgs(`vim %f`, []string{"vim"})
	s.checkArgs(`vim test %f`, []string{"vim", "test"})
}

func (s *DesktopEntryLauncherSuite) TestExecStrWithFilesParam() {
	s.checkArgs(`vim %F`, []string{"vim"})
	s.checkArgs(`vim test %F`, []string{"vim", "test"})

	s.files = []string{FILE0}
	s.checkArgs(`vim %F`, []string{"vim", FILE0})
	s.checkArgs(`vim test %F`, []string{"vim", "test", FILE0})

	s.files = []string{FILE0, FILE1}
	s.checkArgs(`vim %F`, []string{"vim", FILE0, FILE1})
	s.checkArgs(`vim test %F`, []string{"vim", "test", FILE0, FILE1})

	s.files = []string{}
	s.urls = []string{URL0}
	s.checkArgs(`vim %F`, []string{"vim"})
	s.checkArgs(`vim test %F`, []string{"vim", "test"})

	s.urls = []string{URL0, URL1}
	s.checkArgs(`vim %F`, []string{"vim"})
	s.checkArgs(`vim test %F`, []string{"vim", "test"})
}

func (s *DesktopEntryLauncherSuite) TestExecStrWithUrlParam() {
	s.checkArgs(`vim %u`, []string{"vim"})
	s.checkArgs(`vim test %u`, []string{"vim", "test"})

	s.files = []string{FILE0}
	s.checkArgs(`vim %u`, []string{"vim"})
	s.checkArgs(`vim test %u`, []string{"vim", "test"})

	s.files = []string{FILE0, FILE1}
	s.checkArgs(`vim %u`, []string{"vim"})
	s.checkArgs(`vim test %u`, []string{"vim", "test"})

	s.files = []string{}
	s.urls = []string{URL0}
	s.checkArgs(`vim %u`, []string{"vim", URL0})
	s.checkArgs(`vim test %u`, []string{"vim", "test", URL0})

	s.urls = []string{URL0, URL1}
	s.checkArgs(`vim %u`, []string{"vim", URL0})
	s.checkArgs(`vim test %u`, []string{"vim", "test", URL0})
}

func (s *DesktopEntryLauncherSuite) TestExecStrWithUrlsParam() {
	s.checkArgs(`vim %U`, []string{"vim"})
	s.checkArgs(`vim test %U`, []string{"vim", "test"})

	s.files = []string{FILE0}
	s.checkArgs(`vim %U`, []string{"vim"})
	s.checkArgs(`vim test %U`, []string{"vim", "test"})

	s.files = []string{FILE0, FILE1}
	s.checkArgs(`vim %U`, []string{"vim"})
	s.checkArgs(`vim test %U`, []string{"vim", "test"})

	s.files = []string{}
	s.urls = []string{URL0}
	s.checkArgs(`vim %U`, []string{"vim", URL0})
	s.checkArgs(`vim test %U`, []string{"vim", "test", URL0})

	s.urls = []string{URL0, URL1}
	s.checkArgs(`vim %U`, []string{"vim", URL0, URL1})
	s.checkArgs(`vim test %U`, []string{"vim", "test", URL0, URL1})
}

func (s *DesktopEntryLauncherSuite) TestNonValidFieldCodes() {
	s.checkArgs(`vim test %x`, []string{"vim", "test", "%x"})
	s.checkArgs(`vim %x test`, []string{"vim", "%x", "test"})
	s.checkArgs(`vim %x/.vimrc`, []string{"vim", "%x/.vimrc"})

	s.files = []string{FILE0}
	s.checkArgs(`vim test %x`, []string{"vim", "test", "%x"})
	s.checkArgs(`vim %x test`, []string{"vim", "%x", "test"})
	s.checkArgs(`vim %x/.vimrc`, []string{"vim", "%x/.vimrc"})

	s.files = []string{FILE0, FILE1}
	s.checkArgs(`vim test %x`, []string{"vim", "test", "%x"})
	s.checkArgs(`vim %x test`, []string{"vim", "%x", "test"})
	s.checkArgs(`vim %x/.vimrc`, []string{"vim", "%x/.vimrc"})

	s.files = []string{}
	s.urls = []string{URL0}
	s.checkArgs(`vim test %x`, []string{"vim", "test", "%x"})
	s.checkArgs(`vim %x test`, []string{"vim", "%x", "test"})
	s.checkArgs(`vim %x/.vimrc`, []string{"vim", "%x/.vimrc"})

	s.urls = []string{URL0, URL1}
	s.checkArgs(`vim test %x`, []string{"vim", "test", "%x"})
	s.checkArgs(`vim %x test`, []string{"vim", "%x", "test"})
	s.checkArgs(`vim %x/.vimrc`, []string{"vim", "%x/.vimrc"})
}

func (s *DesktopEntryLauncherSuite) TestQuotes() {
	// TODO: https://specifications.freedesktop.org/desktop-entry-spec/latest/ar01s07.html
	s.checkArgs(`"gvim" test`, []string{"gvim", "test"})
	s.checkArgs(`"gvim test"`, []string{"gvim test"})
	s.checkArgs(`vim ~/.vimrc`, []string{"vim", "~/.vimrc"})
	s.checkArgs(`vim '~/.vimrc test'`, []string{"vim", "~/.vimrc test"})
	s.checkArgs(`vim '~/.vimrc " test'`, []string{"vim", `~/.vimrc " test`})
}

func (s *DesktopEntryLauncherSuite) TestEscapeSequences() {
	s.checkArgs(`"gvim test" test2 "test \\" 3"`, []string{"gvim test", "test2", `test " 3`})
	s.checkArgs(`"test \\\\\\\\ \\" moin" test`, []string{`test \\ " moin`, "test"})
	s.checkArgs(`"gvim \\\\\\\\ \\`+"`"+`test\\$"`, []string{`gvim \\ ` + "`" + `test$`})
}

func (s *DesktopEntryLauncherSuite) TestEscapeValidFieldCodes() {
	s.checkArgs(`vim "\\%u" ~/.vimrc`, []string{"vim", "%u", "~/.vimrc"})
}

func TestDesktopEntryLauncher(t *testing.T) {
	suite.Run(t, new(DesktopEntryLauncherSuite))
}
