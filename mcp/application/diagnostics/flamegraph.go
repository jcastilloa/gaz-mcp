package diagnostics

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"

	"github.com/google/pprof/profile"
)

const (
	flamegraphWidth       = 1200
	flamegraphFrameHeight = 24
	flamegraphMargin      = 12
)

type flameNode struct {
	Name     string
	Value    int64
	Children map[string]*flameNode
}

type flameFrame struct {
	Name   string
	Value  int64
	Depth  int
	X      float64
	Width  float64
	Colour string
}

type flamegraphDocument struct {
	Width    int
	Height   int
	Total    int64
	Unit     string
	Frames   []flameFrame
	FrameBox int
}

var flamegraphTemplate = template.Must(template.New("flamegraph").Funcs(template.FuncMap{
	"add": func(left, right int) int { return left + right },
	"sub": func(left, right int) int { return left - right },
	"mul": func(left, right int) int { return left * right },
}).Parse(`<svg xmlns="http://www.w3.org/2000/svg" width="{{.Width}}" height="{{.Height}}" viewBox="0 0 {{.Width}} {{.Height}}">
<style>text{font-family:monospace;font-size:12px;pointer-events:none}.frame:hover rect{stroke:#111;stroke-width:1}</style>
<rect width="100%" height="100%" fill="#fff"/>
<text x="12" y="16">Flame graph — total {{.Total}} {{.Unit}}</text>
{{range .Frames}}<g class="frame"><title>{{.Name}} ({{.Value}} {{$.Unit}})</title><rect x="{{printf "%.2f" .X}}" y="{{mul .Depth $.FrameBox}}" width="{{printf "%.2f" .Width}}" height="{{sub $.FrameBox 2}}" fill="{{.Colour}}"/><text x="{{printf "%.2f" .X}}" y="{{add (mul .Depth $.FrameBox) 16}}">{{.Name}}</text></g>
{{end}}</svg>`))

func renderFlamegraph(parsed *profile.Profile) ([]byte, error) {
	sample, ok := selectedSampleType(parsed)
	if !ok {
		return nil, fmt.Errorf("profile has no sample type")
	}
	root := buildFlameTree(parsed, sample.index)
	if root.Value <= 0 {
		return nil, fmt.Errorf("profile has no samples for %s", sample.Type)
	}
	frames, maxDepth := flattenFlameTree(root)
	document := flamegraphDocument{
		Width: flamegraphWidth, Height: flamegraphMargin*2 + (maxDepth+2)*flamegraphFrameHeight,
		Total: root.Value, Unit: sample.Unit, Frames: frames, FrameBox: flamegraphFrameHeight,
	}
	var output bytes.Buffer
	if err := flamegraphTemplate.Execute(&output, document); err != nil {
		return nil, fmt.Errorf("render flame graph: %w", err)
	}
	return output.Bytes(), nil
}

func buildFlameTree(parsed *profile.Profile, sampleIndex int) *flameNode {
	root := &flameNode{Children: make(map[string]*flameNode)}
	for _, sample := range parsed.Sample {
		if sampleIndex >= len(sample.Value) || sample.Value[sampleIndex] <= 0 {
			continue
		}
		value := sample.Value[sampleIndex]
		root.Value += value
		node := root
		for index := len(sample.Location) - 1; index >= 0; index-- {
			name := locationName(sample.Location[index])
			if name == "" {
				continue
			}
			child := node.Children[name]
			if child == nil {
				child = &flameNode{Name: name, Children: make(map[string]*flameNode)}
				node.Children[name] = child
			}
			child.Value += value
			node = child
		}
	}
	return root
}

func flattenFlameTree(root *flameNode) ([]flameFrame, int) {
	frames := make([]flameFrame, 0)
	maxDepth := 0
	appendFlameFrames(&frames, root, root.Value, 0, float64(flamegraphMargin), float64(flamegraphWidth-flamegraphMargin*2), &maxDepth)
	return frames, maxDepth
}

func appendFlameFrames(frames *[]flameFrame, node *flameNode, total int64, depth int, x, width float64, maxDepth *int) {
	position := x
	for _, child := range orderedChildren(node) {
		childWidth := width * float64(child.Value) / float64(total)
		*frames = append(*frames, flameFrame{Name: child.Name, Value: child.Value, Depth: depth + 1, X: position, Width: childWidth, Colour: frameColour(child.Name)})
		if depth+1 > *maxDepth {
			*maxDepth = depth + 1
		}
		appendFlameFrames(frames, child, child.Value, depth+1, position, childWidth, maxDepth)
		position += childWidth
	}
}

func orderedChildren(node *flameNode) []*flameNode {
	children := make([]*flameNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Value == children[j].Value {
			return children[i].Name < children[j].Name
		}
		return children[i].Value > children[j].Value
	})
	return children
}

func frameColour(name string) string {
	var hash uint32
	for _, character := range name {
		hash = hash*31 + uint32(character)
	}
	return fmt.Sprintf("#%02x%02x%02x", 230, 80+hash%120, 70+(hash/120)%100)
}
