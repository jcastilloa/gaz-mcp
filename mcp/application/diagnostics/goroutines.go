package diagnostics

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var goroutineHeader = regexp.MustCompile(`^goroutine \d+(?:\s+[^\[]+)?\s+\[([^,\]]+)(?:, ([^\]]+))?\]:$`)
var hexAddress = regexp.MustCompile(`0x[0-9a-fA-F]+`)

type parsedGoroutine struct {
	state      string
	blockedFor time.Duration
	stack      []string
}

func AnalyzeGoroutines(dump string) (GoroutineReport, error) {
	items, err := parseGoroutineDump(dump)
	if err != nil {
		return GoroutineReport{}, err
	}

	groups := groupGoroutines(items)
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			if groups[i].BlockedFor == groups[j].BlockedFor {
				return strings.Join(groups[i].Stack, "\n") < strings.Join(groups[j].Stack, "\n")
			}
			return groups[i].BlockedFor > groups[j].BlockedFor
		}
		return groups[i].Count > groups[j].Count
	})
	return GoroutineReport{Total: len(items), Groups: groups}, nil
}

func parseGoroutineDump(dump string) ([]parsedGoroutine, error) {
	dump = strings.ReplaceAll(dump, "\r\n", "\n")
	blocks := strings.Split(strings.TrimSpace(dump), "\n\n")
	items := make([]parsedGoroutine, 0, len(blocks))
	for _, block := range blocks {
		item, ok := parseGoroutineBlock(block)
		if ok {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no goroutines found in debug=2 response")
	}
	return items, nil
}

func parseGoroutineBlock(block string) (parsedGoroutine, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	if len(lines) < 2 {
		return parsedGoroutine{}, false
	}
	matches := goroutineHeader.FindStringSubmatch(lines[0])
	if matches == nil {
		return parsedGoroutine{}, false
	}

	stack := make([]string, 0, len(lines)/2)
	for index := 1; index < len(lines); index += 2 {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			stack = append(stack, normalizeStackLine(line))
		}
	}
	if len(stack) == 0 {
		return parsedGoroutine{}, false
	}
	return parsedGoroutine{state: matches[1], blockedFor: parseBlockedDuration(matches[2]), stack: stack}, true
}

func parseBlockedDuration(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	value = strings.NewReplacer(
		" hours", "h", " hour", "h",
		" minutes", "m", " minute", "m",
		" seconds", "s", " second", "s",
	).Replace(value)
	duration, err := time.ParseDuration(strings.ReplaceAll(value, " ", ""))
	if err != nil {
		return 0
	}
	return duration
}

func normalizeStackLine(line string) string {
	return hexAddress.ReplaceAllString(line, "0xADDR")
}

func groupGoroutines(items []parsedGoroutine) []GoroutineGroup {
	groups := make(map[[32]byte]GoroutineGroup)
	for _, item := range items {
		key := stackKey(item)
		group, exists := groups[key]
		if !exists {
			group = GoroutineGroup{State: item.state, BlockedFor: item.blockedFor, Stack: item.stack}
		}
		group.Count++
		if item.blockedFor > group.BlockedFor {
			group.BlockedFor = item.blockedFor
		}
		groups[key] = group
	}

	result := make([]GoroutineGroup, 0, len(groups))
	for _, group := range groups {
		group.Findings = matchSignatures(group)
		result = append(result, group)
	}
	return result
}

func stackKey(item parsedGoroutine) [32]byte {
	return sha256.Sum256([]byte(item.state + "\n" + strings.Join(item.stack, "\n")))
}
