package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestConsumerGRPCService(t *testing.T) {
	consumerFeature := features.New("Consumer GRPC Service").
		WithLabel("type", "grpc").
		WithLabel("res", "consumer").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client, err := cfg.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			maestroDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "maestro-api", Namespace: "maestro"},
			}
			// wait for the deployment to become at least 50%
			err = wait.For(conditions.New(client.Resources()).ResourceMatch(maestroDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return float64(d.Status.ReadyReplicas)/float64(*d.Spec.Replicas) >= 0.50
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}
			// t.Logf("deployment availability: %.2f%%", float64(maestroDep.Status.ReadyReplicas)/float64(*maestroDep.Spec.Replicas)*100)

			conn := ctx.Value("grpc-connction").(*grpc.ClientConn)
			grpcClient := maestropbv1.NewConsumerServiceClient(conn)

			return context.WithValue(ctx, "grpc-consumer-client", grpcClient)
		}).
		Assess("Should be able to create a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a consumer
			grpcClient := ctx.Value("grpc-consumer-client").(maestropbv1.ConsumerServiceClient)
			pbConsumer, err := grpcClient.Create(ctx, &maestropbv1.ConsumerCreateRequest{
				Labels: []*maestropbv1.ConsumerLabel{
					{
						Key:   "foo",
						Value: "bar",
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("consumer created: %s", pbConsumer.Id)
			consumerID = pbConsumer.Id
			return ctx
		}).
		Assess("Should be able to retrieve a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// retrieve the consumer
			grpcClient := ctx.Value("grpc-consumer-client").(maestropbv1.ConsumerServiceClient)
			pbConsumer, err := grpcClient.Read(ctx, &maestropbv1.ConsumerReadRequest{
				Id: consumerID,
			})
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("consumer retrieved: %s", pbConsumer.Id)

			return ctx
		}).
		Assess("Should be able to update a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// update the consumer
			grpcClient := ctx.Value("grpc-consumer-client").(maestropbv1.ConsumerServiceClient)
			pbConsumer, err := grpcClient.Update(ctx, &maestropbv1.ConsumerUpdateRequest{
				Id: consumerID,
				Labels: []*maestropbv1.ConsumerLabel{
					{
						Key:   "foo",
						Value: "goo",
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			labels := pbConsumer.GetLabels()
			if len(labels) != 1 {
				t.Fatalf("expected label count %d, got %d", 1, len(labels))
			}
			label := labels[0]
			if label.Key != "foo" {
				t.Fatalf("expected label key %s, got %s", "foo", label.Key)
			}
			if label.Value != "goo" {
				t.Fatalf("expected label value %s, got %s", "goo", label.Value)
			}

			t.Logf("consumer updated: %s", pbConsumer.Id)
			return ctx
		}).
		Assess("Update the cluster id for work-agent deployment", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var workAgentDep appsv1.Deployment
			if err := cfg.Client().Resources().Get(ctx, "work-agent", "open-cluster-management-agent", &workAgentDep); err != nil {
				t.Fatal(err)
			}
			// update the cluster id for work-agent deployment
			args := workAgentDep.Spec.Template.Spec.Containers[0].Args
			for i, arg := range args {
				if strings.Contains(arg, "--spoke-cluster-name=") {
					args[i] = fmt.Sprintf("--spoke-cluster-name=%s", consumerID)
					break
				}
			}

			err := cfg.Client().Resources().Update(ctx, &workAgentDep)
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, consumerFeature)
}
