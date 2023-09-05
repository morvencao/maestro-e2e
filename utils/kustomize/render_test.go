package kustomize

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestRender(t *testing.T) {
	// Test
	buf, err := Render(Options{
		KustomizationPath: "tests",
	})
	require.NoError(t, err, "Render()")
	rendered := rendered(t, buf)
	names := containedNames(rendered)
	assert.Equal(t, []string{"foo"}, names, "rendered names")

	labels, _ := GetLabels(buf)
	for labelKey, labelValue := range labels.(map[string]interface{}) {
		assert.Equal(t, "foo", labelKey, "metadata label key")
		assert.Equal(t, "bar", labelValue, "metadata label value")
	}

	str := string(buf)
	pkgLabelPos := strings.Index(str, "foo: bar\n")
	assert.True(t, pkgLabelPos > 0, "foo: bar label should be contained")

}

func containedNames(rendered []map[string]interface{}) (names []string) {
	for _, o := range rendered {
		m := o["metadata"]
		name := ""
		if mm, ok := m.(map[string]interface{}); ok {
			name = mm["name"].(string)
		} else {
			name = m.(map[interface{}]interface{})["name"].(string)
		}
		names = append(names, name)
	}
	return
}

func rendered(t *testing.T, rendered []byte) (r []map[string]interface{}) {
	dec := yaml.NewDecoder(bytes.NewReader(rendered))
	o := map[string]interface{}{}
	var err error
	for ; err == nil; err = dec.Decode(o) {
		require.NoError(t, err)
		if len(o) > 0 {
			r = append(r, o)
			o = map[string]interface{}{}
		}
	}
	if err != io.EOF {
		require.NoError(t, err)
	}
	return
}
