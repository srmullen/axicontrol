// Package svg sanitizes uploaded SVG files: parse, allowlist-walk, and
// re-serialize, so the result is safe to store and to render via an <img>
// tag without corrupting the path/shape geometry axicli reads.
package svg

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/beevik/etree"
)

// ErrNotSVG is returned when the input parses as XML but its root element
// isn't <svg>.
var ErrNotSVG = errors.New("not an SVG document")

// layerOptionOverridePattern matches AxiDraw's per-layer option override
// tokens appended to a layer name, e.g. "+H30" (pen-down height) or "+S25"
// (speed) in a label like "1 black +H30 +S25".
var layerOptionOverridePattern = regexp.MustCompile(`\s*\+[A-Za-z]+-?\d+(?:\.\d+)?`)

// Sanitize parses SVG data and returns a re-serialized copy with dangerous or
// AxiDraw-config-overriding content removed: <script> elements,
// foreignObject-embedded HTML, event-handler attributes (onload, onclick,
// ...), javascript: URIs, and AxiDraw-specific per-layer option overrides.
// Path/shape geometry is left untouched.
func Sanitize(data []byte) ([]byte, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}

	root := doc.Root()
	if root == nil || !strings.EqualFold(root.Tag, "svg") {
		return nil, ErrNotSVG
	}

	sanitizeElement(root)

	out, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize svg: %w", err)
	}
	return out, nil
}

func sanitizeElement(e *etree.Element) {
	for _, child := range e.ChildElements() {
		if isDisallowedElement(child) {
			e.RemoveChild(child)
			continue
		}
		stripDangerousAttrs(child)
		stripLayerOptionOverride(child)
		sanitizeElement(child)
	}
}

func isDisallowedElement(e *etree.Element) bool {
	switch strings.ToLower(e.Tag) {
	case "script", "foreignobject":
		return true
	default:
		return false
	}
}

// stripDangerousAttrs removes event-handler attributes (onload, onclick, ...)
// and javascript: URIs in href/xlink:href, in place.
func stripDangerousAttrs(e *etree.Element) {
	kept := e.Attr[:0]
	for _, a := range e.Attr {
		key := strings.ToLower(a.Key)
		if strings.HasPrefix(key, "on") {
			continue
		}
		if key == "href" && isJavascriptURI(a.Value) {
			continue
		}
		kept = append(kept, a)
	}
	e.Attr = kept
}

func isJavascriptURI(v string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "javascript:")
}

// stripLayerOptionOverride removes AxiDraw per-layer option override tokens
// from an Inkscape layer's label, leaving the base layer name (e.g. "1
// black") that axicontrol's layer-discovery relies on intact.
func stripLayerOptionOverride(e *etree.Element) {
	if !strings.EqualFold(e.Tag, "g") {
		return
	}
	if e.SelectAttrValue("inkscape:groupmode", "") != "layer" {
		return
	}
	label := e.SelectAttr("inkscape:label")
	if label == nil {
		return
	}
	label.Value = strings.TrimSpace(layerOptionOverridePattern.ReplaceAllString(label.Value, ""))
}
