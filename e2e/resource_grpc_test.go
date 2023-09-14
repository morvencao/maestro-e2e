package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	maestropbv1 "github.com/kube-orchestra/maestro/proto/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestResourceGRPCService(t *testing.T) {
	resourceFeature := features.New("Resource GRPC Service").
		WithLabel("type", "grpc").
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

			conn := ctx.Value("grpc-connction").(*grpc.ClientConn)
			grpcClient := maestropbv1.NewResourceServiceClient(conn)

			return context.WithValue(ctx, "grpc-resource-client", grpcClient)
		}).
		Assess("should be able to create a resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a resource
			grpcClient := ctx.Value("grpc-resource-client").(maestropbv1.ResourceServiceClient)
			nginxDeployJSON := []byte(`
{
	"apiVersion": "apps/v1",
	"kind": "Deployment",
	"metadata": {
		"name": "nginx2",
		"namespace": "default"
	},
	"spec": {
		"replicas": 1,
		"selector": {
			"matchLabels": {
				"app": "nginx2"
			}
		},
		"template": {
			"metadata": {
				"labels": {
					"app": "nginx2"
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

			obj := map[string]interface{}{}
			err := json.Unmarshal(nginxDeployJSON, &obj)
			if err != nil {
				t.Fatal(err)
			}
			objStruct, err := structpb.NewStruct(obj)
			if err != nil {
				t.Fatal(err)
			}
			pbResource, err := grpcClient.Create(ctx, &maestropbv1.ResourceCreateRequest{
				ConsumerId: consumerID,
				Object:     objStruct,
			})
			if err != nil {
				t.Fatal(err)
			}

			nginxDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "nginx2", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(nginxDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas == 1
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource created: %s", pbResource.Id)
			resourceID = pbResource.Id
			return ctx
		}).
		Assess("should be able to retrieve the resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// retrieve the resource
			grpcClient := ctx.Value("grpc-resource-client").(maestropbv1.ResourceServiceClient)
			resReadReq := &maestropbv1.ResourceReadRequest{
				Id: resourceID,
			}

			pbResource := &maestropbv1.Resource{}
			err := wait.For(func(context.Context) (done bool, err error) {
				pbResource, err = grpcClient.Read(ctx, resReadReq)
				if err != nil {
					t.Fatal(err)
				}

				spec := pbResource.Object.Fields["spec"]
				replicas := spec.GetStructValue().Fields["replicas"]
				if replicas.GetNumberValue() != float64(1) {
					return false, nil
				}

				contentStatus := pbResource.Status.Fields["contentStatus"]
				readyReplicas := contentStatus.GetStructValue().Fields["readyReplicas"]
				if readyReplicas.GetNumberValue() != float64(1) {
					return false, nil
				}

				return true, nil
			}, wait.WithTimeout(time.Minute*2), wait.WithInterval(time.Second*5))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource retrieved: %s", pbResource.Id)
			return ctx
		}).
		Assess("should be able to update the resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// update the resource
			grpcClient := ctx.Value("grpc-resource-client").(maestropbv1.ResourceServiceClient)
			nginxDeployJSON := []byte(`
{
	"apiVersion": "apps/v1",
	"kind": "Deployment",
	"metadata": {
		"name": "nginx2",
		"namespace": "default"
	},
	"spec": {
		"replicas": 2,
		"selector": {
			"matchLabels": {
				"app": "nginx2"
			}
		},
		"template": {
			"metadata": {
				"labels": {
					"app": "nginx2"
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

			obj := map[string]interface{}{}
			err := json.Unmarshal(nginxDeployJSON, &obj)
			if err != nil {
				t.Fatal(err)
			}
			objStruct, err := structpb.NewStruct(obj)
			if err != nil {
				t.Fatal(err)
			}

			pbResource, err := grpcClient.Update(ctx, &maestropbv1.ResourceUpdateRequest{
				Id:     resourceID,
				Object: objStruct,
			})
			if err != nil {
				t.Fatal(err)
			}

			nginxDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "nginx2", Namespace: "default"},
			}

			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(nginxDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas == 2
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource updated: %s", pbResource.Id)
			return ctx
		}).
		Assess("should be able to retrieve the updated resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// retrieve the resource
			grpcClient := ctx.Value("grpc-resource-client").(maestropbv1.ResourceServiceClient)
			resReadReq := &maestropbv1.ResourceReadRequest{
				Id: resourceID,
			}

			pbResource := &maestropbv1.Resource{}
			err := wait.For(func(context.Context) (done bool, err error) {
				pbResource, err = grpcClient.Read(ctx, resReadReq)
				if err != nil {
					t.Fatal(err)
				}

				spec := pbResource.Object.Fields["spec"]
				replicas := spec.GetStructValue().Fields["replicas"]
				if replicas.GetNumberValue() != float64(2) {
					return false, nil
				}

				contentStatus := pbResource.Status.Fields["contentStatus"]
				readyReplicas := contentStatus.GetStructValue().Fields["readyReplicas"]
				if readyReplicas.GetNumberValue() != float64(2) {
					return false, nil
				}

				return true, nil
			}, wait.WithTimeout(time.Minute*2), wait.WithInterval(time.Second*5))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("resource retrieved: %s", pbResource.Id)
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, resourceFeature)
}
