package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	maestropbv1 "github.com/kube-orchestra/maestro/proto/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestConsumerRESTAPI(t *testing.T) {
	consumerFeature := features.New("Consumer REST API").
		WithLabel("type", "rest").
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
			t.Logf("maestro deployment availability: %.2f%%", float64(maestroDep.Status.ReadyReplicas)/float64(*maestroDep.Spec.Replicas)*100)
			return ctx
		}).
		Assess("Should be able to create a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a consumer
			requestURL := fmt.Sprintf("%s/%s", maestroRESTBaseURL, "v1/consumers")
			jsonBody := []byte(`{"name": "Test", "labels": [{"key": "baz", "value": "qux" }]}`)
			bodyReader := bytes.NewReader(jsonBody)
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

			consumer := &maestropbv1.Consumer{}
			err = json.Unmarshal(bodyBytes, consumer)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("consumer created: %s", consumer.Id)
			consumerID = consumer.Id
			return ctx
		}).
		Assess("Should be able to retrieve a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// retrieve the consumer
			requestURL := fmt.Sprintf("%s/%s/%s", maestroRESTBaseURL, "v1/consumers", consumerID)
			req, err := http.NewRequest(http.MethodGet, requestURL, nil)
			if err != nil {
				t.Fatal(err)
			}

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

			consumer := &maestropbv1.Consumer{}
			err = json.Unmarshal(bodyBytes, consumer)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("consumer retrieved: %s", consumer.Id)

			return ctx
		}).
		Assess("Should be able to update a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// update the consumer
			requestURL := fmt.Sprintf("%s/%s/%s", maestroRESTBaseURL, "v1/consumers", consumerID)
			jsonBody := []byte(`{"labels": [{"key": "baz", "value": "quux" }]}`)
			bodyReader := bytes.NewReader(jsonBody)
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

			consumer := &maestropbv1.Consumer{}
			err = json.Unmarshal(bodyBytes, consumer)
			if err != nil {
				t.Fatal(err)
			}

			labels := consumer.GetLabels()
			if len(labels) != 1 {
				t.Fatalf("expected label count %d, got %d", 1, len(labels))
			}
			label := labels[0]
			if label.Key != "baz" {
				t.Fatalf("expected label key %s, got %s", "baz", label.Key)
			}
			if label.Value != "quux" {
				t.Fatalf("expected label value %s, got %s", "quux", label.Value)
			}

			t.Logf("consumer updated: %s", consumer.Id)

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

			expectedWorkAgentDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "work-agent", Namespace: "open-cluster-management-agent"},
			}
			// wait for the deployment to become at least 50%
			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(expectedWorkAgentDep, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return float64(d.Status.ReadyReplicas)/float64(*d.Spec.Replicas) >= 0.50
			}), wait.WithTimeout(time.Minute*2))
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("work-agent deployment availability: %.2f%%", float64(expectedWorkAgentDep.Status.ReadyReplicas)/float64(*expectedWorkAgentDep.Spec.Replicas)*100)

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, consumerFeature)
}
