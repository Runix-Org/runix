// Package table owns deterministic table records and their ordering/selection state.
package table

import "sort"

// Record is one row in the table.
type Record struct {
	ID, Title string
	Value     int
	Status    string
}

// State owns rows, sort order, and selection independently of Qt.
type State struct {
	Records       []Record
	SelectedIndex int
	SelectedID    string
	ActivatedID   string
	SortColumn    int
	Descending    bool
}

// New copies records so callers can retain their input slice.
func New(records []Record) *State {
	s := &State{Records: append([]Record(nil), records...), SelectedIndex: -1}
	s.sortRecords()
	if len(s.Records) > 0 {
		s.selectIndex(0)
	}
	return s
}

func (s *State) sortRecords() {
	sort.Slice(s.Records, func(i, j int) bool { return s.before(s.Records[i], s.Records[j]) })
}

func (s *State) before(a, b Record) bool {
	cmp := 0
	switch s.SortColumn {
	case 0:
		if a.Title < b.Title {
			cmp = -1
		} else if a.Title > b.Title {
			cmp = 1
		}
	case 1:
		if a.Value < b.Value {
			cmp = -1
		} else if a.Value > b.Value {
			cmp = 1
		}
	case 2:
		if a.Status < b.Status {
			cmp = -1
		} else if a.Status > b.Status {
			cmp = 1
		}
	case 3:
		if a.ID < b.ID {
			cmp = -1
		} else if a.ID > b.ID {
			cmp = 1
		}
	}
	if cmp == 0 {
		if a.ID < b.ID {
			cmp = -1
		} else if a.ID > b.ID {
			cmp = 1
		}
	}
	if s.Descending {
		return cmp > 0
	}
	return cmp < 0
}

func (s *State) selectIndex(index int) {
	s.SelectedIndex, s.SelectedID = index, ""
	if index >= 0 && index < len(s.Records) {
		s.SelectedID = s.Records[index].ID
	}
}

// Select selects a row by current index.
func (s *State) Select(index int) bool {
	if index < 0 || index >= len(s.Records) {
		return false
	}
	s.selectIndex(index)
	return true
}

// Activate captures a stable ID, independent of later sorting.
func (s *State) Activate(id string) bool {
	if !s.hasID(id) {
		return false
	}
	s.ActivatedID = id
	return true
}

// Sort sorts by column, toggling direction for a repeated column.
func (s *State) Sort(column int) bool {
	if column < 0 || column > 3 {
		return false
	}
	if column == s.SortColumn {
		s.Descending = !s.Descending
	} else {
		s.SortColumn, s.Descending = column, false
	}
	selected := s.SelectedID
	s.sortRecords()
	s.restoreSelection(selected, false)
	return true
}

// UpdatedPosition returns the row's destination after its value and status change.
func (s *State) UpdatedPosition(id string) (oldRow, newRow int, ok bool) {
	oldRow = -1
	var changed Record
	for i, record := range s.Records {
		if record.ID == id {
			oldRow, changed = i, record
			break
		}
	}
	if oldRow < 0 {
		return 0, 0, false
	}
	changed.Value++
	if changed.Status == "Active" {
		changed.Status = "Inactive"
	} else {
		changed.Status = "Active"
	}
	for i, record := range s.Records {
		if record.ID != id && s.before(changed, record) {
			if i > oldRow {
				i--
			}
			return oldRow, i, true
		}
	}
	return oldRow, len(s.Records) - 1, true
}

// Update changes the row and inserts it into its sorted position.
func (s *State) Update(id string) bool {
	oldRow, newRow, ok := s.UpdatedPosition(id)
	if !ok {
		return false
	}
	record := s.Records[oldRow]
	record.Value++
	if record.Status == "Active" {
		record.Status = "Inactive"
	} else {
		record.Status = "Active"
	}
	if newRow > oldRow {
		copy(s.Records[oldRow:newRow], s.Records[oldRow+1:newRow+1])
	} else if newRow < oldRow {
		copy(s.Records[newRow+1:oldRow+1], s.Records[newRow:oldRow])
	}
	s.Records[newRow] = record
	s.restoreSelection(s.SelectedID, false)
	return true
}

// Remove removes ID and selects the closest row when the selected row is removed.
func (s *State) Remove(id string) bool {
	for i, record := range s.Records {
		if record.ID != id {
			continue
		}
		selected := s.SelectedID
		s.Records = append(s.Records[:i], s.Records[i+1:]...)
		if selected == id {
			if i >= len(s.Records) {
				i = len(s.Records) - 1
			}
			s.selectIndex(i)
		} else {
			s.restoreSelection(selected, true)
		}
		return true
	}
	return false
}

func (s *State) restoreSelection(id string, fallback bool) {
	for i := range s.Records {
		if s.Records[i].ID == id && id != "" {
			s.selectIndex(i)
			return
		}
	}
	if fallback && len(s.Records) > 0 {
		s.selectIndex(0)
	} else if len(s.Records) == 0 {
		s.selectIndex(-1)
	}
}

func (s *State) hasID(id string) bool {
	for _, record := range s.Records {
		if record.ID == id {
			return true
		}
	}
	return false
}
