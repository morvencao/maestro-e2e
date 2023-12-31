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

func TestManifestRESTAPI(t *testing.T) {
	manifestFeature := features.New("Manifest REST API").
		WithLabel("type", "rest").
		WithLabel("res", "manifest").
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
		Assess("should be able to post a manifest", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a manifest
			requestURL := fmt.Sprintf("%s/%s", maestroRESTBaseURL, "v1/cloudevents")
			webDeployCEJSON := []byte(fmt.Sprintf(`
{
	"id": "835e075f-cd45-43b4-9793-3184e34e836b",
	"specversion": "1.0",
	"time": "2023-08-30T17:31:00Z",
	"datacontenttype": "application/json",
	"source": "maestro",
	"type": "io.open-cluster-management.works.v1alpha1.manifests.spec.create_request",
	"clustername": "%s",
	"resourceid": "748d04ad-887a-44dd-b788-da1373227072",
	"resourceversion": "1",
	"data": {
		"manifest": {
			"apiVersion": "apps/v1",
			"kind": "Deployment",
			"metadata": {
				"name": "web1",
				"namespace": "default"
			},
			"spec": {
				"replicas": 1,
				"selector": {
				"matchLabels": {
					"app": "web1"
				}
				},
				"template": {
				"metadata": {
					"labels": {
					"app": "web1"
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
		}
	}
}`, consumerID))

			bodyReader := bytes.NewReader(webDeployCEJSON)
			req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
			if err != nil {
				t.Fatal(err)
			}

			req.Header.Set("Content-Type", "application/x-cloudevents")
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

			ceSendResp := &maestropbv1.CloudEventSendResponse{}
			err = protojson.Unmarshal(bodyBytes, ceSendResp)
			if err != nil {
				t.Fatal(err)
			}

			webDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web1", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(webDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas == 1
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("manifest created")
			return ctx
		}).
		Assess("should be able to update the manifest", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// update the manifest
			requestURL := fmt.Sprintf("%s/%s", maestroRESTBaseURL, "v1/cloudevents")
			webDeployCEJSON := []byte(fmt.Sprintf(`
{
	"id": "97801f5f-e283-4ae0-9a5c-e0a7e8356537",
	"specversion": "1.0",
	"time": "2023-08-31T17:31:00Z",
	"datacontenttype": "application/json",
	"source": "maestro",
	"type": "io.open-cluster-management.works.v1alpha1.manifests.spec.update_request",
	"clustername": "%s",
	"resourceid": "748d04ad-887a-44dd-b788-da1373227072",
	"resourceversion": "2",
	"data": {
		"manifest": {
			"apiVersion": "apps/v1",
			"kind": "Deployment",
			"metadata": {
				"name": "web1",
				"namespace": "default"
			},
			"spec": {
				"replicas": 2,
				"selector": {
				"matchLabels": {
					"app": "web1"
				}
				},
				"template": {
				"metadata": {
					"labels": {
					"app": "web1"
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
		}
	}
}`, consumerID))

			bodyReader := bytes.NewReader(webDeployCEJSON)
			req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
			if err != nil {
				t.Fatal(err)
			}

			req.Header.Set("Content-Type", "application/x-cloudevents")
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

			ceSendResp := &maestropbv1.CloudEventSendResponse{}
			err = protojson.Unmarshal(bodyBytes, ceSendResp)
			if err != nil {
				t.Fatal(err)
			}

			webDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web1", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(webDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas == 2
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("manifest updated")
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, manifestFeature)
}
