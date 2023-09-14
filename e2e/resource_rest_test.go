package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	maestropbv1 "github.com/kube-orchestra/maestro/proto/api/v1"
	"google.golang.org/protobuf/encoding/protojson"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var resourceID = ""

func TestResourceRESTAPI(t *testing.T) {
	resourceFeature := features.New("Resource REST API").
		WithLabel("type", "rest").
		WithLabel("res", "resource").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			if consumerID == "" {
				t.Fatal("consumerID is empty")
			}

			client, err := cfg.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			workAgentDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "work-agent", Namespace: "open-cluster-management-agent"},
			}
			// wait for the deployment to become at least 50%
			err = wait.For(conditions.New(client.Resources()).ResourceMatch(workAgentDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return float64(d.Status.ReadyReplicas)/float64(*d.Spec.Replicas) >= 0.50
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}
			// t.Logf("deployment availability: %.2f%%", float64(workAgentDep.Status.ReadyReplicas)/float64(*workAgentDep.Spec.Replicas)*100)
			return ctx
		}).
		Assess("should be able to create a resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a resource
			requestURL := fmt.Sprintf("%s/%s/%s/%s", maestroRESTBaseURL, "v1/consumers", consumerID, "resources")
			nginxDeployJSON := []byte(`
{
	"apiVersion": "apps/v1",
	"kind": "Deployment",
	"metadata": {
		"name": "nginx1",
		"namespace": "default"
	},
	"spec": {
		"replicas": 1,
		"selector": {
			"matchLabels": {
				"app": "nginx1"
			}
		},
		"template": {
			"metadata": {
				"labels": {
					"app": "nginx1"
				}
			},
			"spec": {
				"containers": [
					{
						"image": "quay.io/jitesoft/nginx",
						"imagePullPolicy": "IfNotPresent",
						"name": "nginx"
					}
				]
			}
		}
	}
}`)

			bodyReader := bytes.NewReader(nginxDeployJSON)
			req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
			if err != nil {
				t.Fatal(err)
			}

			req.Header.Set("Content-Type", "application/json")
			httpClient := ctx.Value("http-client").(*http.Client)
			resp, err := httpClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			resource := &maestropbv1.Resource{}
			err = protojson.Unmarshal(bodyBytes, resource)
			if err != nil {
				t.Fatal(err)
			}

			nginxDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "nginx1", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(nginxDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas == 1
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource created: %s", resource.Id)
			resourceID = resource.Id
			return ctx
		}).
		Assess("should be able to retrieve the resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// retrieve the resource
			requestURL := fmt.Sprintf("%s/%s/%s", maestroRESTBaseURL, "v1/resources", resourceID)
			req, err := http.NewRequest(http.MethodGet, requestURL, nil)
			if err != nil {
				t.Fatal(err)
			}

			httpClient := ctx.Value("http-client").(*http.Client)
			resource := &maestropbv1.Resource{}
			err = wait.For(func(context.Context) (done bool, err error) {
				resp, err := httpClient.Do(req)
				if err != nil {
					return false, err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return false, nil
				}

				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					return false, err
				}

				err = protojson.Unmarshal(bodyBytes, resource)
				if err != nil {
					return false, err
				}

				spec := resource.Object.Fields["spec"]
				replicas := spec.GetStructValue().Fields["replicas"]
				if replicas.GetNumberValue() != float64(1) {
					return false, nil
				}

				contentStatus := resource.Status.Fields["contentStatus"]
				readyReplicas := contentStatus.GetStructValue().Fields["readyReplicas"]
				if readyReplicas.GetNumberValue() != float64(1) {
					return false, nil
				}

				return true, nil
			}, wait.WithTimeout(time.Minute*2), wait.WithInterval(time.Second*5))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource retrieved: %s", resource.Id)
			return ctx
		}).
		Assess("should be able to update the resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// update the resource
			requestURL := fmt.Sprintf("%s/%s/%s", maestroRESTBaseURL, "v1/resources", resourceID)
			nginxDeployJSON := []byte(`
{
	"apiVersion": "apps/v1",
	"kind": "Deployment",
	"metadata": {
		"name": "nginx1",
		"namespace": "default"
	},
	"spec": {
		"replicas": 2,
		"selector": {
			"matchLabels": {
				"app": "nginx1"
			}
		},
		"template": {
			"metadata": {
				"labels": {
					"app": "nginx1"
				}
			},
			"spec": {
				"containers": [
					{
						"image": "quay.io/jitesoft/nginx",
						"imagePullPolicy": "IfNotPresent",
						"name": "nginx"
					}
				]
			}
		}
	}
}`)
			bodyReader := bytes.NewReader(nginxDeployJSON)
			req, err := http.NewRequest(http.MethodPut, requestURL, bodyReader)
			if err != nil {
				t.Fatal(err)
			}

			req.Header.Set("Content-Type", "application/json")
			httpClient := ctx.Value("http-client").(*http.Client)
			resp, err := httpClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			resource := &maestropbv1.Resource{}
			err = protojson.Unmarshal(bodyBytes, resource)
			if err != nil {
				t.Fatal(err)
			}

			nginxDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "nginx1", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(nginxDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas == 2
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource updated: %s", resource.Id)
			return ctx
		}).
		Assess("should be able to retrieve the updated resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// retrieve the resource
			requestURL := fmt.Sprintf("%s/%s/%s", maestroRESTBaseURL, "v1/resources", resourceID)
			req, err := http.NewRequest(http.MethodGet, requestURL, nil)
			if err != nil {
				t.Fatal(err)
			}

			httpClient := ctx.Value("http-client").(*http.Client)
			resource := &maestropbv1.Resource{}
			err = wait.For(func(context.Context) (done bool, err error) {
				resp, err := httpClient.Do(req)
				if err != nil {
					return false, err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return false, nil
				}

				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					return false, err
				}

				err = protojson.Unmarshal(bodyBytes, resource)
				if err != nil {
					return false, err
				}

				spec := resource.Object.Fields["spec"]
				replicas := spec.GetStructValue().Fields["replicas"]
				if replicas.GetNumberValue() != float64(2) {
					return false, nil
				}

				contentStatus := resource.Status.Fields["contentStatus"]
				readyReplicas := contentStatus.GetStructValue().Fields["readyReplicas"]
				if readyReplicas.GetNumberValue() != float64(2) {
					return false, nil
				}

				return true, nil
			}, wait.WithTimeout(time.Minute*2), wait.WithInterval(time.Second*5))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource retrieved: %s", resource.Id)
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, resourceFeature)
}
