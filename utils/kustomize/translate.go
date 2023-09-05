package kustomize

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	yamlSeparator = "---"
)

func ToObjects(manifests []byte) ([]*unstructured.Unstructured, error) {
	yamls := strings.Split(string(manifests), yamlSeparator)
	objs := make([]*unstructured.Unstructured, 0, len(yamls))
	for _, f := range yamls {
		if len(strings.TrimSpace(f)) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		err := yaml.Unmarshal([]byte(f), obj)
		if err != nil {
			return objs, err
		}

		objs = append(objs, obj)
	}

	return objs, nil
}
