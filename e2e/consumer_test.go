package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	maestrov1 "github.com/kube-orchestra/maestro/proto/api/v1"
)

const baseURL = "http://127.0.0.1:31330"

func TestConsumerAPI(t *testing.T) {
	consumerFeature := features.New("Consumer API").
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
			return ctx
		}).
		Assess("Should be able to create a consumer", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// create a consumer
			requestURL := fmt.Sprintf("%s/%s", baseURL, "v1/consumers")
			jsonBody := []byte(`{"name": "Test", "labels": [{"key": "k1", "value": "v1" }]}`)
			bodyReader := bytes.NewReader(jsonBody)
			req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
			if err != nil {
				t.Fatal(err)
			}

			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
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

			consumer := &maestrov1.Consumer{}
			err = json.Unmarshal(bodyBytes, consumer)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("consumer created: %s", consumer.Id)

			// retrieve the consumer
			requestURL = fmt.Sprintf("%s/%s/%s", baseURL, "v1/consumers", consumer.Id)
			req, err = http.NewRequest(http.MethodGet, requestURL, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
			}

			bodyBytes, err = io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			err = json.Unmarshal(bodyBytes, consumer)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("consumer retrieved: %s", consumer.Id)

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// do nothing
			return ctx
		}).Feature()

	testenv.Test(t, consumerFeature)
}
