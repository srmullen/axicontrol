package svg

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// layerNumberPattern matches AxiDraw's numeric-prefix layer-naming
// convention: a layer name begins with an integer, optionally followed by
// a hyphen/whitespace separator and then other text (e.g. "1 black",
// "5-red"). Group 1 captures just the number; the full match additionally
// consumes the separator, which StripNumberPrefix relies on to strip
// exactly the part parseLeadingLayerNumber parsed.
var layerNumberPattern = regexp.MustCompile(`^\s*(\d+)[-\s]*`)

// Layer is one auto-discovered AxiDraw layer (CONTEXT.md's Layer entry):
// its number, and the distinct raw inkscape:label text(s), in document
// order, of every SVG layer group that shares it. Multiple <g> elements can
// collapse into one Layer number; the UI must show their original label
// text(s), not just the bare number.
type Layer struct {
	Number int
	Labels []string
}

// DiscoverLayers parses data for Inkscape layer groups and returns each
// distinct layer number found, in ascending order, together with the
// distinct raw label text(s) that collapsed into it, per AxiDraw's
// numeric-prefix layer-naming convention. Multiple layers can share one
// number (e.g. "5-red" and "5-outlines") and plot together in a single
// axicli invocation; an SVG with no numbered layers returns an empty slice.
func DiscoverLayers(data []byte) ([]Layer, error) {
	doc, err := parseSVGDocument(data)
	if err != nil {
		return nil, err
	}
	root := doc.Root()

	byNumber := map[int]*Layer{}
	var order []int
	collectLayers(root, byNumber, &order)
	sort.Ints(order)

	layers := make([]Layer, len(order))
	for i, n := range order {
		layers[i] = *byNumber[n]
	}
	return layers, nil
}

// StripNumberPrefix removes label's leading numeric-prefix layer number
// (the convention DiscoverLayers parses) and any immediately following
// hyphen/whitespace separator, leaving just the descriptive remainder —
// e.g. "5-red" → "red", "1 black" → "black". A label with nothing after
// its number strips to "".
func StripNumberPrefix(label string) string {
	return layerNumberPattern.ReplaceAllString(label, "")
}

// Numbers returns just layers' layer numbers, discarding their labels —
// for a layers-mode Job submission, which only needs which layers exist
// (one Pass per number), not their label text.
func Numbers(layers []Layer) []int {
	numbers := make([]int, len(layers))
	for i, l := range layers {
		numbers[i] = l.Number
	}
	return numbers
}

func collectLayers(e *etree.Element, byNumber map[int]*Layer, order *[]int) {
	for _, child := range e.ChildElements() {
		if isLayerGroup(child) {
			label := child.SelectAttrValue("inkscape:label", "")
			if n, ok := parseLeadingLayerNumber(label); ok {
				addLayerLabel(byNumber, order, n, label)
			}
		}
		collectLayers(child, byNumber, order)
	}
}

func isLayerGroup(e *etree.Element) bool {
	return strings.EqualFold(e.Tag, "g") && e.SelectAttrValue("inkscape:groupmode", "") == "layer"
}

// IsolateLayer parses data and returns a re-serialized copy with every
// Inkscape layer group removed except the one(s) whose parsed leading
// number equals number (see DiscoverLayers — multiple groups can share one
// number and are all kept together). Non-layer content (defs, root
// attributes, ungrouped geometry) is left untouched, so the isolated
// document still renders at the same size. A layer group nested inside a
// kept layer is itself still just "another layer group" and is stripped if
// its own number doesn't match — sublayers don't inherit their parent's
// fate. A number with no matching layer strips everything, returning a
// valid but empty document rather than erroring, since the caller is
// expected to have already validated number against DiscoverLayers.
func IsolateLayer(data []byte, number int) ([]byte, error) {
	doc, err := parseSVGDocument(data)
	if err != nil {
		return nil, err
	}

	stripOtherLayers(doc.Root(), number)

	out, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize svg: %w", err)
	}
	return out, nil
}

// parseSVGDocument parses data as XML and validates its root is an <svg>
// element, the identical preamble DiscoverLayers and IsolateLayer both need
// — the former only ever reads from the result, the latter also needs the
// *etree.Document itself back (via Root()) to re-serialize after mutating.
func parseSVGDocument(data []byte) (*etree.Document, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}

	root := doc.Root()
	if root == nil || !strings.EqualFold(root.Tag, "svg") {
		return nil, ErrNotSVG
	}
	return doc, nil
}

func stripOtherLayers(e *etree.Element, number int) {
	for _, child := range e.ChildElements() {
		if isLayerGroup(child) {
			label := child.SelectAttrValue("inkscape:label", "")
			n, ok := parseLeadingLayerNumber(label)
			if !ok || n != number {
				e.RemoveChild(child)
				continue
			}
		}
		stripOtherLayers(child, number)
	}
}

// addLayerLabel records label against layer number n, creating n's entry
// (and appending it to order, the encounter order sorted into DiscoverLayers's
// final ascending-by-number result) the first time n is seen, and
// appending label only if it isn't already present — the "distinct" in
// DiscoverLayers's per-layer label list.
func addLayerLabel(byNumber map[int]*Layer, order *[]int, n int, label string) {
	l, exists := byNumber[n]
	if !exists {
		l = &Layer{Number: n}
		byNumber[n] = l
		*order = append(*order, n)
	}
	if !slices.Contains(l.Labels, label) {
		l.Labels = append(l.Labels, label)
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
