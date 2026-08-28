package bench

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type sample struct {
	name   string
	labels map[string]string
	value  float64
}

type snapshot struct{ samples []sample }

func parsePrometheus(r io.Reader) (*snapshot, error) {
	result := &snapshot{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		head, raw, ok := splitSample(line)
		if !ok {
			return nil, fmt.Errorf("metrics line %d: malformed sample", lineNo)
		}
		name, labels, err := parseHead(head)
		if err != nil {
			return nil, fmt.Errorf("metrics line %d: %w", lineNo, err)
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("metrics line %d: %w", lineNo, err)
		}
		result.samples = append(result.samples, sample{name, labels, value})
	}
	return result, scanner.Err()
}

func splitSample(line string) (string, string, bool) {
	quoted, escaped, depth := false, false, 0
	for i, ch := range line {
		if quoted {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				quoted = false
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '{':
			depth++
		case '}':
			depth--
		case ' ', '\t':
			if depth == 0 {
				fields := strings.Fields(line[i:])
				if len(fields) > 0 {
					return line[:i], fields[0], true
				}
			}
		}
	}
	return "", "", false
}

func parseHead(head string) (string, map[string]string, error) {
	open := strings.IndexByte(head, '{')
	if open < 0 {
		if head == "" {
			return "", nil, fmt.Errorf("empty metric name")
		}
		return head, nil, nil
	}
	if open == 0 || !strings.HasSuffix(head, "}") {
		return "", nil, fmt.Errorf("malformed labels")
	}
	labels := map[string]string{}
	raw := head[open+1 : len(head)-1]
	for len(strings.TrimSpace(raw)) > 0 {
		// Prometheus exposition allows a trailing comma after the last label.
		raw = strings.TrimLeft(raw, " \t,")
		if raw == "" {
			break
		}
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return "", nil, fmt.Errorf("malformed label")
		}
		key := strings.TrimSpace(raw[:eq])
		raw = strings.TrimSpace(raw[eq+1:])
		if !strings.HasPrefix(raw, "\"") {
			return "", nil, fmt.Errorf("malformed label %q", key)
		}
		end, escaped := -1, false
		for i := 1; i < len(raw); i++ {
			if escaped {
				escaped = false
				continue
			}
			if raw[i] == '\\' {
				escaped = true
				continue
			}
			if raw[i] == '"' {
				end = i
				break
			}
		}
		if end < 0 {
			return "", nil, fmt.Errorf("unterminated label %q", key)
		}
		value, err := strconv.Unquote(raw[:end+1])
		if err != nil {
			return "", nil, err
		}
		labels[key] = value
		raw = raw[end+1:]
		if strings.TrimSpace(raw) != "" && !strings.HasPrefix(strings.TrimSpace(raw), ",") {
			return "", nil, fmt.Errorf("malformed separator")
		}
	}
	return head[:open], labels, nil
}

func (s *snapshot) total(name string, labels map[string]string) float64 {
	if s == nil {
		return 0
	}
	var total float64
	for _, v := range s.samples {
		if v.name != name {
			continue
		}
		match := true
		for key, want := range labels {
			if v.labels[key] != want {
				match = false
				break
			}
		}
		if match {
			total += v.value
		}
	}
	return total
}

func acceptedCounts(s *snapshot) Counts {
	return Counts{
		Spans:        s.total("fanout_ingest_rows_total", map[string]string{"signal": "spans"}),
		LogRecords:   s.total("fanout_ingest_rows_total", map[string]string{"signal": "logs"}),
		MetricPoints: s.total("fanout_ingest_rows_total", map[string]string{"signal": "metrics"}),
	}
}

func subtractCounts(after, before Counts) Counts {
	return Counts{after.Spans - before.Spans, after.LogRecords - before.LogRecords, after.MetricPoints - before.MetricPoints}
}
