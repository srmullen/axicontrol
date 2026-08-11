package svg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverLayersReturnsSortedDistinctNumbers(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="2 red"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="1 black"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="5-outlines"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="5-fill"><path d="M0 0 L1 1"/></g>
</svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 5}, Numbers(layers))
}

func TestDiscoverLayersIgnoresNonLayerGroups(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:label="3 not a layer"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="unnamed layer"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="4 blue"><path d="M0 0 L1 1"/></g>
</svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []int{4}, Numbers(layers))
}

func TestDiscoverLayersReturnsEmptyForNoNumberedLayers(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Empty(t, layers)
}

func TestDiscoverLayersFindsNestedLayers(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="1 black">
    <g inkscape:groupmode="layer" inkscape:label="3 sublayer"><path d="M0 0 L1 1"/></g>
  </g>
</svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, Numbers(layers))
}

func TestDiscoverLayersRejectsNonSVGRoot(t *testing.T) {
	_, err := DiscoverLayers([]byte(`<html><body>hello</body></html>`))
	require.ErrorIs(t, err, ErrNotSVG)
}

func TestDiscoverLayersRejectsNonXML(t *testing.T) {
	_, err := DiscoverLayers([]byte("not xml at all\x00\x01"))
	require.Error(t, err)
}

func TestDiscoverLayersCollectsDistinctLabelsPerNumber(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="5-red"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="5-outlines"><path d="M0 0 L1 1"/></g>
</svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Len(t, layers, 1)
	require.Equal(t, 5, layers[0].Number)
	require.Equal(t, []string{"5-red", "5-outlines"}, layers[0].Labels, "both raw labels that collapsed into layer 5 must be kept, in document order")
}

func TestDiscoverLayersDedupesIdenticalLabelsForTheSameNumber(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="5-red"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="5-red"><path d="M0 0 L1 1"/></g>
</svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Len(t, layers, 1)
	require.Equal(t, []string{"5-red"}, layers[0].Labels)
}

func TestDiscoverLayersKeepsDistinctLabelsSeparateAcrossNumbers(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="1 black"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="2 red"><path d="M0 0 L1 1"/></g>
</svg>`

	layers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []Layer{
		{Number: 1, Labels: []string{"1 black"}},
		{Number: 2, Labels: []string{"2 red"}},
	}, layers)
}

func TestStripNumberPrefixRemovesLeadingNumberAndSeparator(t *testing.T) {
	require.Equal(t, "red", StripNumberPrefix("5-red"))
	require.Equal(t, "black", StripNumberPrefix("1 black"))
	require.Equal(t, "outlines", StripNumberPrefix("5-outlines"))
}

func TestStripNumberPrefixOfBareNumberIsEmpty(t *testing.T) {
	require.Equal(t, "", StripNumberPrefix("5"))
}

func TestNumbersExtractsJustTheNumbersDiscardingLabels(t *testing.T) {
	layers := []Layer{
		{Number: 1, Labels: []string{"1 black"}},
		{Number: 5, Labels: []string{"5-red", "5-outlines"}},
	}

	require.Equal(t, []int{1, 5}, Numbers(layers))
}

func TestNumbersOfEmptyLayersIsEmpty(t *testing.T) {
	require.Empty(t, Numbers(nil))
}

func TestIsolateLayerKeepsOnlyTheChosenLayerGroup(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="10" height="10">
  <g inkscape:groupmode="layer" inkscape:label="1 black"><rect width="5" height="5"/></g>
  <g inkscape:groupmode="layer" inkscape:label="2 red"><rect width="5" height="5" x="5" y="5"/></g>
</svg>`

	out, err := IsolateLayer([]byte(in), 2)

	require.NoError(t, err)
	layers, err := DiscoverLayers(out)
	require.NoError(t, err)
	require.Equal(t, []int{2}, Numbers(layers))
	require.Contains(t, string(out), `x="5" y="5"`, "layer 2's own geometry must survive")
	require.NotContains(t, string(out), `inkscape:label="1 black"`, "layer 1's group must be stripped entirely")
}

func TestIsolateLayerKeepsBothGroupsThatShareTheChosenNumber(t *testing.T) {
	out, err := IsolateLayer([]byte(multiLabelLayeredSVGForIsolate), 5)

	require.NoError(t, err)
	layers, err := DiscoverLayers(out)
	require.NoError(t, err)
	require.Equal(t, []string{"5-red", "5-outlines"}, layers[0].Labels, "both groups collapsing into the chosen number must be kept")
}

const multiLabelLayeredSVGForIsolate = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="10" height="10">
  <g inkscape:groupmode="layer" inkscape:label="5-red"><rect width="5" height="5"/></g>
  <g inkscape:groupmode="layer" inkscape:label="5-outlines"><rect width="5" height="5" x="5" y="5"/></g>
</svg>`

func TestIsolateLayerStripsUnnumberedAndUnlabeledLayerGroups(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:label="3 not a layer"><rect width="1" height="1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="unnamed layer"><rect width="1" height="1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="4 blue"><rect width="1" height="1"/></g>
</svg>`

	out, err := IsolateLayer([]byte(in), 4)

	require.NoError(t, err)
	require.NotContains(t, string(out), "unnamed layer")
	require.Contains(t, string(out), "4 blue")
	require.Contains(t, string(out), "3 not a layer", "a plain (non-layer) group isn't 'another layer' and must survive")
}

func TestIsolateLayerOfANonExistentNumberStripsEverything(t *testing.T) {
	out, err := IsolateLayer([]byte(layeredSVGForIsolate), 9)

	require.NoError(t, err)
	layers, err := DiscoverLayers(out)
	require.NoError(t, err)
	require.Empty(t, layers)
}

const layeredSVGForIsolate = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="10" height="10">
  <g inkscape:groupmode="layer" inkscape:label="1 black"><rect width="5" height="5"/></g>
  <g inkscape:groupmode="layer" inkscape:label="2 red"><rect width="5" height="5" x="5" y="5"/></g>
</svg>`

func TestIsolateLayerPreservesRootAttributesAndNonLayerContent(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="42" height="42" viewBox="0 0 42 42">
  <defs><style>.a{fill:red}</style></defs>
  <g inkscape:groupmode="layer" inkscape:label="1 black"><rect width="5" height="5"/></g>
</svg>`

	out, err := IsolateLayer([]byte(in), 1)

	require.NoError(t, err)
	require.Contains(t, string(out), `width="42"`)
	require.Contains(t, string(out), `viewBox="0 0 42 42"`)
	require.Contains(t, string(out), "<defs>")
}

func TestIsolateLayerStripsADifferentlyNumberedSublayerNestedInsideTheChosenLayer(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="1 black">
    <g inkscape:groupmode="layer" inkscape:label="3 sublayer"><rect width="1" height="1"/></g>
  </g>
</svg>`

	out, err := IsolateLayer([]byte(in), 1)

	require.NoError(t, err)
	layers, err := DiscoverLayers(out)
	require.NoError(t, err)
	require.Equal(t, []int{1}, Numbers(layers), "layer 1 itself must survive, but its nested layer-3 sublayer is 'another layer group' and must be stripped")
}

func TestIsolateLayerRejectsNonSVGRoot(t *testing.T) {
	_, err := IsolateLayer([]byte(`<html><body>hello</body></html>`), 1)
	require.ErrorIs(t, err, ErrNotSVG)
}
