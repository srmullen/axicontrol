package svg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const maliciousSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns:xlink="http://www.w3.org/1999/xlink" width="100" height="100">
  <script>alert('xss')</script>
  <g inkscape:groupmode="layer" inkscape:label="1 black +H30 +S25">
    <path d="M10 10 L90 90" onload="alert('xss')" stroke="black"/>
  </g>
  <foreignObject width="50" height="50"><body xmlns="http://www.w3.org/1999/xhtml"><p>html</p></body></foreignObject>
  <a xlink:href="javascript:alert('xss')"><circle cx="5" cy="5" r="2"/></a>
</svg>`

func TestSanitizeStripsScriptTag(t *testing.T) {
	out, err := Sanitize([]byte(maliciousSVG))
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(out)), "<script")
	require.NotContains(t, string(out), "alert")
}

func TestSanitizeStripsEventHandlerAttribute(t *testing.T) {
	out, err := Sanitize([]byte(maliciousSVG))
	require.NoError(t, err)
	require.NotContains(t, string(out), "onload")
}

func TestSanitizeStripsLayerOptionOverride(t *testing.T) {
	out, err := Sanitize([]byte(maliciousSVG))
	require.NoError(t, err)
	require.NotContains(t, string(out), "+H30")
	require.NotContains(t, string(out), "+S25")
	require.Contains(t, string(out), `inkscape:label="1 black"`)
}

func TestSanitizeStripsForeignObject(t *testing.T) {
	out, err := Sanitize([]byte(maliciousSVG))
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(out)), "foreignobject")
	require.NotContains(t, string(out), "<p>html</p>")
}

func TestSanitizeStripsJavascriptURI(t *testing.T) {
	out, err := Sanitize([]byte(maliciousSVG))
	require.NoError(t, err)
	require.NotContains(t, string(out), "javascript:")
}

func TestSanitizePreservesGeometry(t *testing.T) {
	out, err := Sanitize([]byte(maliciousSVG))
	require.NoError(t, err)
	require.Contains(t, string(out), `d="M10 10 L90 90"`)
	require.Contains(t, string(out), `cx="5"`)
	require.Contains(t, string(out), `cy="5"`)
	require.Contains(t, string(out), `r="2"`)
}

func TestSanitizePreservesPlainLayerLabel(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="2 red"><path d="M0 0 L1 1"/></g>
</svg>`
	out, err := Sanitize([]byte(in))
	require.NoError(t, err)
	require.Contains(t, string(out), `inkscape:label="2 red"`)
}

func TestSanitizeRejectsNonXML(t *testing.T) {
	_, err := Sanitize([]byte("not xml at all, just some bytes\x00\x01"))
	require.Error(t, err)
}

func TestSanitizeRejectsNonSVGRoot(t *testing.T) {
	_, err := Sanitize([]byte(`<html><body>hello</body></html>`))
	require.ErrorIs(t, err, ErrNotSVG)
}

func TestSanitizeRejectsEmptyInput(t *testing.T) {
	_, err := Sanitize([]byte(""))
	require.Error(t, err)
}
