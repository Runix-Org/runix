package desktop

import "strings"

type mimeStorage struct {
	mimeTypes map[string][]*DesktopEntry
}

func newMimeStorage() *mimeStorage {
	return &mimeStorage{
		mimeTypes: map[string][]*DesktopEntry{},
	}
}

func (ms *mimeStorage) GetByMimeType(mimeType string) []*DesktopEntry {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return ms.mimeTypes[mimeType]
}

func (ms *mimeStorage) addDesktopFile(types []string, dfile *DesktopEntry) {
	for _, mimeType := range types {
		mimeType = strings.ToLower(strings.TrimSpace(mimeType))
		if mimeType != "" {
			ms.mimeTypes[mimeType] = append(ms.mimeTypes[mimeType], dfile)
		}
	}
}
