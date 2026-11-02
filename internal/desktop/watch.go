package desktop

import (
	"os"
	"path/filepath"
	"strings"
)

// Each physical directory has one watch and reference counts for relevant children.
// Empty child means the directory itself belongs to the reachable application tree.
type watchGraph map[string]map[string]int

func (graph *watchGraph) add(directory, child string) {
	if graph == nil {
		return
	}
	if (*graph)[directory] == nil {
		(*graph)[directory] = make(map[string]int)
	}
	(*graph)[directory][child]++
}

// observe follows every link component explicitly, including broken targets. Only
// the next component of an ancestor is relevant, not unrelated sibling activity.
func (graph *watchGraph) observe(path string) {
	if graph == nil || !filepath.IsAbs(path) {
		return
	}
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))[1:]
	current := string(filepath.Separator)
	links := 0
	for len(parts) > 0 {
		name := parts[0]
		parts = parts[1:]
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			current = filepath.Dir(current)
			continue
		}
		graph.add(current, name)
		next := filepath.Join(current, name)
		info, err := os.Lstat(next)
		if err != nil {
			return
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 255 {
				return
			}
			target, readErr := os.Readlink(next)
			if readErr != nil {
				return
			}
			if filepath.IsAbs(target) {
				current = string(filepath.Separator)
			}
			parts = append(strings.Split(target, string(filepath.Separator)), parts...)
			continue
		}
		if len(parts) > 0 && !info.IsDir() {
			return
		}
		current = next
	}
}

func (graph *watchGraph) directory(path string) {
	if graph == nil {
		return
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		graph.add(resolved, "")
	}
}

func (graph watchGraph) relevant(path string) bool {
	children := graph[filepath.Dir(path)]
	return children[""] > 0 || children[filepath.Base(path)] > 0 || graph[path] != nil
}
