package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	cepbv2 "github.com/cloudevents/sdk-go/binding/format/protobuf/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	maestropbv1 "github.com/kube-orchestra/maestro/proto/api/v1"
	"google.golang.org/grpc"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestManifestGRPCService(t *testing.T) {
	manifestFeature := features.New("Manifest GRPC Service").
		WithLabel("type", "grpc").
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

			conn := ctx.Value("grpc-connction").(*grpc.ClientConn)
			grpcClient := maestropbv1.NewCloudEventsServiceClient(conn)

			return context.WithValue(ctx, "grpc-manifest-client", grpcClient)
		}).
		Assess("should be able to post a manifest", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a manifest
			grpcClient := ctx.Value("grpc-manifest-client").(maestropbv1.CloudEventsServiceClient)
			webDeployCEJSON := []byte(fmt.Sprintf(`
{
	"id": "8f61b49e-e51d-4c17-a8e2-1e1d89a77e2c",
	"specversion": "1.0",
	"time": "2023-09-01T17:31:00Z",
	"datacontenttype": "application/json",
	"source": "maestro",
	"type": "io.open-cluster-management.works.v1alpha1.manifests.spec.create_request",
	"clustername": "%s",
	"resourceid": "8a178798-f445-459c-ad66-6645246ec811",
	"resourceversion": "1",
	"data": {
		"manifest": {
			"apiVersion": "apps/v1",
			"kind": "Deployment",
			"metadata": {
				"name": "web2",
				"namespace": "default"
			},
			"spec": {
				"replicas": 1,
				"selector": {
				"matchLabels": {
					"app": "web2"
				}
				},
				"template": {
				"metadata": {
					"labels": {
					"app": "web2"
					}
				},
				"spec": {
					"containers": [
					{
						"image": "nginxinc/nginx-unprivileged",
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

			evt := &event.Event{}
			err := json.Unmarshal(webDeployCEJSON, evt)
			if err != nil {
				log.Fatal(err)
			}

			pbEvt, err := cepbv2.ToProto(evt)
			if err != nil {
				log.Fatal(err)
			}

			pbCESendResp, err := grpcClient.Send(ctx, pbEvt)
			if err != nil {
				t.Fatal(err)
			}

			webDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web2", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(webDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return *d.Spec.Replicas == 1
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("manifest created: %s", pbCESendResp.Status)
			return ctx
		}).
		Assess("should be able to update the manifest", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// update the manifest
			grpcClient := ctx.Value("grpc-manifest-client").(maestropbv1.CloudEventsServiceClient)
			webDeployCEJSON := []byte(fmt.Sprintf(`
{
	"id": "b89d65c9-954a-4718-8d0b-6ae70122ada7",
	"specversion": "1.0",
	"time": "2023-09-02T17:31:00Z",
	"datacontenttype": "application/json",
	"source": "maestro",
	"type": "io.open-cluster-management.works.v1alpha1.manifests.spec.update_request",
	"clustername": "%s",
	"resourceid": "8a178798-f445-459c-ad66-6645246ec811",
	"resourceversion": "2",
	"data": {
		"manifest": {
			"apiVersion": "apps/v1",
			"kind": "Deployment",
			"metadata": {
				"name": "web2",
				"namespace": "default"
			},
			"spec": {
				"replicas": 2,
				"selector": {
				"matchLabels": {
					"app": "web2"
				}
				},
				"template": {
				"metadata": {
					"labels": {
					"app": "web2"
					}
				},
				"spec": {
					"containers": [
					{
						"image": "nginxinc/nginx-unprivileged",
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

			evt := &event.Event{}
			err := json.Unmarshal(webDeployCEJSON, evt)
			if err != nil {
				log.Fatal(err)
			}

			pbEvt, err := cepbv2.ToProto(evt)
			if err != nil {
				log.Fatal(err)
			}

			pbCESendResp, err := grpcClient.Send(ctx, pbEvt)
			if err != nil {
				t.Fatal(err)
			}

			webDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web2", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(webDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return *d.Spec.Replicas == 2
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("manifest updated: %s", pbCESendResp.Status)
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, manifestFeature)
}
