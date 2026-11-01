package desktop

import (
	"fmt"
	"strings"
)

// CompileExec consumes the already desktop-decoded value. Substitutions never
// become command syntax. The caller must use the selected catalog entry.
func CompileExec(entry Entry) ([]string, error) {
	command := entry.Exec.Decoded
	var argv []string
	fileCodes := 0
	for position := 0; position < len(command); {
		if command[position] == ' ' {
			position++
			continue
		}
		quoted := command[position] == '"'
		if quoted {
			position++
		}
		var word strings.Builder
		closed := !quoted
		for position < len(command) {
			character := command[position]
			if quoted && character == '"' {
				position++
				closed = true
				break
			}
			if !quoted && character == ' ' {
				break
			}
			if quoted && character == '\\' {
				position++
				if position == len(command) || !strings.ContainsRune("\"`$\\", rune(command[position])) {
					return nil, fmt.Errorf("invalid escape in Exec")
				}
				character = command[position]
			} else if (!quoted && strings.ContainsRune("\t\n\r\"'\\><~|&;$*?#()`", rune(character))) ||
				(quoted && strings.ContainsRune("`$", rune(character))) || character == 0 {
				return nil, fmt.Errorf("reserved character in Exec must be quoted or escaped")
			}
			word.WriteByte(character)
			position++
		}
		if !closed || (position < len(command) && command[position] != ' ') {
			return nil, fmt.Errorf("use whole-argument double quotes in Exec")
		}
		value := word.String()
		if len(argv) == 0 {
			if value == "" || strings.ContainsRune(value, '=') {
				return nil, fmt.Errorf("invalid executable in Exec")
			}
			for index := 0; index < len(value); index++ {
				if value[index] == '%' {
					index++
					if index == len(value) || value[index] != '%' {
						return nil, fmt.Errorf("executable in Exec must not contain field codes")
					}
				}
			}
			value = strings.ReplaceAll(value, "%%", "%")
			argv = append(argv, value)
			continue
		}
		expanded, keep, err := expandExecArgument(value, quoted, entry, &fileCodes)
		if err != nil {
			return nil, err
		}
		if keep {
			argv = append(argv, expanded...)
		}
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("no executable in Exec")
	}
	return argv, nil
}

func expandExecArgument(value string, quoted bool, entry Entry, fileCodes *int) ([]string, bool, error) {
	var result strings.Builder
	removed := false
	for position := 0; position < len(value); position++ {
		if value[position] != '%' {
			result.WriteByte(value[position])
			continue
		}
		position++
		if position == len(value) {
			return nil, false, fmt.Errorf("incomplete field code at the end of Exec")
		}
		code := value[position]
		if quoted && code != '%' {
			return nil, false, fmt.Errorf("field codes in Exec must not be quoted")
		}
		if strings.ContainsRune("FUi", rune(code)) && len(value) != 2 {
			return nil, false, fmt.Errorf("field code %%%c in Exec must be a separate argument", code)
		}
		switch code {
		case '%':
			result.WriteByte('%')
		case 'c':
			result.WriteString(entry.Name.Selected)
		case 'k':
			result.WriteString(entry.Source.Path)
		case 'i':
			if entry.Icon.Selected == "" {
				return nil, false, nil
			}
			return []string{"--icon", entry.Icon.Selected}, true, nil
		case 'f', 'F', 'u', 'U':
			*fileCodes++
			if *fileCodes > 1 {
				return nil, false, fmt.Errorf("use at most one file or URL field code in Exec")
			}
			removed = true
		case 'd', 'D', 'n', 'N', 'v', 'm':
			removed = true
		default:
			return nil, false, fmt.Errorf("unknown Exec field code %%%c", code)
		}
	}
	if removed && result.Len() == 0 {
		return nil, false, nil
	}
	return []string{result.String()}, true, nil
}
