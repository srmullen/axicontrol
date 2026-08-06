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

	numbers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 5}, numbers)
}

func TestDiscoverLayersIgnoresNonLayerGroups(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:label="3 not a layer"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="unnamed layer"><path d="M0 0 L1 1"/></g>
  <g inkscape:groupmode="layer" inkscape:label="4 blue"><path d="M0 0 L1 1"/></g>
</svg>`

	numbers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []int{4}, numbers)
}

func TestDiscoverLayersReturnsEmptyForNoNumberedLayers(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`

	numbers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Empty(t, numbers)
}

func TestDiscoverLayersFindsNestedLayers(t *testing.T) {
	const in = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g inkscape:groupmode="layer" inkscape:label="1 black">
    <g inkscape:groupmode="layer" inkscape:label="3 sublayer"><path d="M0 0 L1 1"/></g>
  </g>
</svg>`

	numbers, err := DiscoverLayers([]byte(in))

	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, numbers)
}

func TestDiscoverLayersRejectsNonSVGRoot(t *testing.T) {
	_, err := DiscoverLayers([]byte(`<html><body>hello</body></html>`))
	require.ErrorIs(t, err, ErrNotSVG)
}

func TestDiscoverLayersRejectsNonXML(t *testing.T) {
	_, err := DiscoverLayers([]byte("not xml at all\x00\x01"))
	require.Error(t, err)
}
