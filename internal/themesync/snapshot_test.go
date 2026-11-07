package themesync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"runix/internal/theme"

	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	suite.Suite
	original []byte
	license  []byte
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) SetupTest() {
	catalog, err := theme.Bundled()
	s.Require().NoError(err)
	entry, found := catalog.Lookup(theme.DefaultDarkID)
	s.Require().True(found)
	s.original = []byte(entry.Original)
	s.license = []byte("MIT License\nCopyright (c) 2024 Raycast Technologies Ltd\n")
}

func (s *SnapshotSuite) snapshot(sources map[string][]byte) *Snapshot {
	snapshot, err := New(sources, s.license, theme.UpstreamRevision)
	s.Require().NoError(err)
	return snapshot
}

func (s *SnapshotSuite) changedAuthor() []byte {
	var fields map[string]json.RawMessage
	s.Require().NoError(json.Unmarshal(s.original, &fields))
	fields["author"] = json.RawMessage(`"New author"`)
	fields["customMetadata"] = json.RawMessage(`{"credit":"Retain this too"}`)
	data, err := json.Marshal(fields)
	s.Require().NoError(err)
	return data
}

func (s *SnapshotSuite) TestInventoryAndReplacementRetainOriginalMetadata() {
	old := s.snapshot(map[string][]byte{"same.json": s.original, "removed.json": s.original})
	updated := s.changedAuthor()
	candidate := s.snapshot(map[string][]byte{"same.json": updated, "added.json": s.original})
	report := candidate.Inventory(old)
	s.Require().Len(report.Added, 1)
	s.Equal("added.json", report.Added[0].Path)
	s.Require().Len(report.Removed, 1)
	s.Equal("removed.json", report.Removed[0].Path)
	s.Require().Len(report.Changed, 1)
	s.Equal("Iliyan Kupenov", report.Changed[0].Before.Author)
	s.Equal("New author", report.Changed[0].After.Author)
	s.NotEqual(report.Changed[0].Before.SHA256, report.Changed[0].After.SHA256)
	destination := filepath.Join(s.T().TempDir(), "bundle")
	s.Require().NoError(old.Write(destination))
	s.Require().NoError(candidate.Write(destination))
	data, err := os.ReadFile(filepath.Join(destination, "themes", "same.json"))
	s.Require().NoError(err)
	s.Equal(updated, data)
	s.NoFileExists(filepath.Join(destination, "themes", "removed.json"))
	s.FileExists(filepath.Join(destination, "themes", "added.json"))
	reread, err := ReadBundle(destination)
	s.Require().NoError(err)
	s.Empty(candidate.Inventory(reread).Changed)
	license, err := os.ReadFile(filepath.Join(destination, "LICENSE"))
	s.Require().NoError(err)
	s.Equal(s.license, license)
}

func (s *SnapshotSuite) TestInvalidCandidateCannotMutateExistingBundle() {
	snapshot := s.snapshot(map[string][]byte{"same.json": s.original})
	destination := filepath.Join(s.T().TempDir(), "bundle")
	s.Require().NoError(snapshot.Write(destination))
	manifest, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	s.Require().NoError(err)
	// A valid file followed by an invalid file must not cause partial replacement.
	snapshot.Sources["a-valid.json"] = s.changedAuthor()
	snapshot.Sources["z-invalid.json"] = []byte(`{"version":2}`)
	s.ErrorContains(snapshot.Write(destination), "z-invalid.json: version:")
	after, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	s.Require().NoError(err)
	s.Equal(manifest, after)
	original, err := os.ReadFile(filepath.Join(destination, "themes", "same.json"))
	s.Require().NoError(err)
	s.Equal(s.original, original)
	s.NoFileExists(filepath.Join(destination, "themes", "a-valid.json"))
}

func (s *SnapshotSuite) TestUnrelatedDestinationDataIsNotDeleted() {
	snapshot := s.snapshot(map[string][]byte{"same.json": s.original})
	destination := filepath.Join(s.T().TempDir(), "bundle")
	s.Require().NoError(snapshot.Write(destination))
	unrelated := filepath.Join(destination, "keep.txt")
	s.Require().NoError(os.WriteFile(unrelated, []byte("user data"), 0o600))
	s.ErrorContains(snapshot.Write(destination), "unrelated file keep.txt")
	data, err := os.ReadFile(unrelated)
	s.Require().NoError(err)
	s.Equal("user data", string(data))
}

func (s *SnapshotSuite) TestOverlapAndUnverifiedArchiveAreRejected() {
	root := s.T().TempDir()
	s.ErrorContains(rejectOverlap(root, filepath.Join(root, "bundle")), "must not overlap")
	s.ErrorContains(rejectOverlap(filepath.Join(root, "nested"), root), "must not overlap")
	alias := filepath.Join(root, "alias")
	s.Require().NoError(os.Symlink(root, alias))
	s.ErrorContains(rejectOverlap(root, filepath.Join(alias, "new")), "must not overlap")
	archive := filepath.Join(root, "unverified.tar.gz")
	s.Require().NoError(os.WriteFile(archive, s.original, 0o600))
	_, err := ReadArchive(archive)
	s.ErrorContains(err, "SHA-256 does not match")
}
