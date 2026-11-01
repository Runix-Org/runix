package desktop

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ExecSuite struct{ suite.Suite }

func TestExecSuite(t *testing.T) { suite.Run(t, new(ExecSuite)) }

func (s *ExecSuite) TestArgumentsAndSinglePassExpansion() {
	entry := Entry{
		Name:   Text{Selected: "Приложение %i with spaces"},
		Icon:   Text{Selected: "original icon %c"},
		Source: Source{Path: "/logical path/app.desktop"},
	}
	for _, test := range []struct {
		command string
		want    []string
	}{
		{
			`app "" "a b" "literal ' ~ * ? # ; & | < > ( )"`,
			[]string{"app", "", "a b", "literal ' ~ * ? # ; & | < > ( )"},
		},
		{"app \"a\\\\b\" \"\\$HOME\\`\"", []string{"app", `a\b`, "$HOME`"}},
		{`app %% "%%" %c --name=%c %k %i %F`, []string{
			"app", "%", "%", entry.Name.Selected, "--name=" + entry.Name.Selected,
			entry.Source.Path, "--icon", entry.Icon.Selected,
		}},
		{`app %d %D %n %N %v %m --file=%f`, []string{"app", "--file="}},
		{`/usr/bin/flatpak run --branch=stable --arch=x86_64 --command=app ` +
			`--file-forwarding org.example.App @@u %U @@`, []string{
			"/usr/bin/flatpak", "run", "--branch=stable", "--arch=x86_64", "--command=app",
			"--file-forwarding", "org.example.App", "@@u", "@@",
		}},
	} {
		s.Run(test.command, func() {
			entry.Exec.Decoded = test.command
			argv, err := CompileExec(entry)
			s.Require().NoError(err)
			s.Equal(test.want, argv)
		})
	}
	entry.Icon.Selected = ""
	entry.Exec.Decoded = "app %i %u"
	argv, err := CompileExec(entry)
	s.Require().NoError(err)
	s.Equal([]string{"app"}, argv)
}

func (s *ExecSuite) TestInvalidCommands() {
	for _, command := range []string{
		"", `""`, `a=b arg`, `app "unfinished`, `app a"b"`, `app "a"b`,
		`app 'shell quotes'`, `app $HOME`, `app ~/a`, `app *.txt`, `app a;b`,
		`app a\ b`, `app "\q"`, `app "$HOME"`, `app "` + "`" + `date` + "`" + `"`,
		`app %`, `app %Z`, `app "%c"`, `app x%F`, `app %Utail`, `app --icon=%i`,
		`app %f %U`, `app %u%f`, "app\targ", "app a\x00b",
	} {
		s.Run(command, func() {
			_, err := CompileExec(Entry{Exec: ExecValue{Decoded: command}})
			s.Error(err)
		})
	}
}

func (s *ExecSuite) TestDesktopEscapesAreDecodedOnlyOnce() {
	entry, err := Parse([]byte("[Desktop Entry]\nType=Application\nName=Example\nExec="+
		`app "a\\\\b" "\\$HOME"`), testSource, Environment{})
	s.Require().NoError(err)
	argv, err := CompileExec(entry)
	s.Require().NoError(err)
	s.Equal([]string{"app", `a\b`, "$HOME"}, argv)
}
