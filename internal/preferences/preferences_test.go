package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

type storeSuite struct{ suite.Suite }

func TestStoreSuite(t *testing.T) { suite.Run(t, new(storeSuite)) }

func (s *storeSuite) TestConcurrentOwnersPreserveIndependentValues() {
	root := s.T().TempDir()
	const workers = 12
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("owner-%d", i)
			errors <- New(root).Update(func(values map[string]json.RawMessage) error {
				values[key] = json.RawMessage(fmt.Sprintf("%d", i))
				return nil
			})
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		s.Require().NoError(err)
	}
	values, err := New(root).Read()
	s.Require().NoError(err)
	for i := range workers {
		s.Equal(fmt.Sprintf("%d", i), string(values[fmt.Sprintf("owner-%d", i)]))
	}
}

func (s *storeSuite) TestCorruptAndSymlinkPreferencesAreNotReplaced() {
	root := s.T().TempDir()
	path := filepath.Join(root, "runix", "config.json")
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	original := []byte("{broken")
	s.Require().NoError(os.WriteFile(path, original, 0o600))
	s.Error(New(root).Update(func(map[string]json.RawMessage) error { return nil }))
	got, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal(original, got)
	other := filepath.Join(root, "other")
	s.Require().NoError(os.WriteFile(other, []byte("{}"), 0o600))
	s.Require().NoError(os.Remove(path))
	s.Require().NoError(os.Symlink(other, path))
	s.Error(New(root).Update(func(map[string]json.RawMessage) error { return nil }))
	got, err = os.ReadFile(other)
	s.Require().NoError(err)
	s.Equal("{}", string(got))
}
