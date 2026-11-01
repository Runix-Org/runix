package main

import (
	"fmt"

	"runix/internal/search"
	"runix/internal/table"
)

// Deliberate title collisions exercise stable IDs and cross-field ranking.
func searchFixture(icons []labIcon) []search.Record {
	names := [...]string{"Atlas", "Bridge", "Console", "Delta", "Editor", "Files", "Graph", "Help"}
	categories := [...]string{"Application", "Document", "Command", "Bookmark"}
	records := make([]search.Record, 50_000)
	for index := range records {
		icon := icons[index%len(icons)]
		records[index] = search.Record{
			ID:       fmt.Sprintf("record-%05d", index),
			Title:    fmt.Sprintf("%s %03d", names[index%len(names)], index%100),
			Subtitle: fmt.Sprintf("Workspace %02d project %03d · %s", index%17, index%113, icon.label),
			Category: categories[(index/len(names))%len(categories)],
			IconName: icon.name,
			IconPath: icon.path,
		}
	}
	return records
}

func tableFixture() []table.Record {
	names := [...]string{"Atlas", "Bridge", "Console", "Delta"}
	statuses := [...]string{"Ready", "Running", "Stopped"}
	values := [...]int{2, 10, 100, 0, 25, 1000, 7}
	records := make([]table.Record, 10_000)
	for index := range records {
		records[index] = table.Record{
			ID:     fmt.Sprintf("table-%05d", index),
			Title:  fmt.Sprintf("%s %03d", names[index%len(names)], index%100),
			Value:  values[index%len(values)],
			Status: statuses[index%len(statuses)],
		}
	}
	return records
}
