package dude

import (
	"bufio"
	"strings"
)

// ParseDetailPrint decodes a RouterOS `print detail` style output.
func ParseDetailPrint(out string) []map[string]string {
	var records []map[string]string
	var current map[string]string

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "Flags:") {
			continue
		}

		if isRecordStart(line) {
			if current != nil && len(current) > 0 {
				records = append(records, current)
			}
			current = map[string]string{}
			line = stripIndexAndFlags(line)
		} else if current == nil {
			continue
		}

		parseKV(line, current)
	}
	if current != nil && len(current) > 0 {
		records = append(records, current)
	}
	return records
}

func isRecordStart(line string) bool {
	t := strings.TrimLeft(line, " \t")
	if t == "" {
		return false
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	if i < len(t) && t[i] != ' ' {
		return false
	}
	return true
}

func stripIndexAndFlags(line string) string {
	t := strings.TrimLeft(line, " \t")
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	t = t[i:]
	t = strings.TrimLeft(t, " \t")
	for {
		if len(t) >= 2 && t[1] == ' ' && isLetter(t[0]) {
			t = t[2:]
			t = strings.TrimLeft(t, " \t")
			continue
		}
		break
	}
	return t
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func parseKV(line string, dst map[string]string) {
	i := 0
	n := len(line)
	for i < n {
		for i < n && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= n {
			return
		}
		ks := i
		for i < n && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i >= n || line[i] != '=' {
			return
		}
		key := strings.TrimSpace(line[ks:i])
		i++
		if i >= n {
			dst[key] = ""
			return
		}
		var val string
		if line[i] == '"' {
			i++
			vs := i
			for i < n && line[i] != '"' {
				i++
			}
			val = line[vs:i]
			if i < n {
				i++
			}
		} else {
			vs := i
			for i < n && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			val = line[vs:i]
		}
		if key != "" {
			dst[key] = val
		}
	}
}

// ParseSimplePrint handles non-detail `print` output.
func ParseSimplePrint(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "Flags:") {
			continue
		}
		fields := strings.Fields(t)
		if len(fields) > 0 {
			isNum := true
			for _, b := range fields[0] {
				if b < '0' || b > '9' {
					isNum = false
					break
				}
			}
			if isNum {
				fields = fields[1:]
			}
		}
		if len(fields) > 0 {
			rows = append(rows, fields)
		}
	}
	return rows
}
