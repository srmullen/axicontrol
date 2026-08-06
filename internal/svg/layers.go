package svg

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// layerNumberPattern matches AxiDraw's numeric-prefix layer-naming
// convention: a layer name begins with an integer, optionally followed by
// other text (e.g. "1 black", "5-red").
var layerNumberPattern = regexp.MustCompile(`^\s*(\d+)`)

// DiscoverLayers parses data for Inkscape layer groups and returns the
// distinct layer numbers found, in ascending order, per AxiDraw's
// numeric-prefix layer-naming convention. Multiple layers can share one
// number (e.g. "5-red" and "5-outlines") and plot together in a single
// axicli invocation; an SVG with no numbered layers returns an empty slice.
func DiscoverLayers(data []byte) ([]int, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}

	root := doc.Root()
	if root == nil || !strings.EqualFold(root.Tag, "svg") {
		return nil, ErrNotSVG
	}

	seen := map[int]bool{}
	var numbers []int
	collectLayerNumbers(root, seen, &numbers)
	sort.Ints(numbers)
	return numbers, nil
}

func collectLayerNumbers(e *etree.Element, seen map[int]bool, numbers *[]int) {
	for _, child := range e.ChildElements() {
		if strings.EqualFold(child.Tag, "g") && child.SelectAttrValue("inkscape:groupmode", "") == "layer" {
			if n, ok := parseLeadingLayerNumber(child.SelectAttrValue("inkscape:label", "")); ok && !seen[n] {
				seen[n] = true
				*numbers = append(*numbers, n)
			}
		}
		collectLayerNumbers(child, seen, numbers)
	}
}

func parseLeadingLayerNumber(label string) (int, bool) {
	m := layerNumberPattern.FindStringSubmatch(label)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}
