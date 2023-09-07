package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	"github.com/morvencao/maestro-e2e/utils/kustomize"
)

var (
	testenv env.Environment
)

const (
	dbEndpoint         = "http://127.0.0.1:31310"
	maestroRESTBaseURL = "http://127.0.0.1:31330"
	maestroGPRCBaseURL = "127.0.0.1:31320"
)

func TestMain(m *testing.M) {
	cfg, _ := envconf.NewFromFlags()
	if os.Getenv("REAL_CLUSTER") == "true" {
		path := conf.ResolveKubeConfigFile()
		cfg = cfg.WithKubeconfigFile(path)
		testenv = env.NewWithConfig(cfg)

		testenv.Setup(
			installComponent("../manifests/mqtt-broker"),
			installComponent("../manifests/work-agent"),
			installComponent("../manifests/dynamodb"),
			installComponent("../manifests/maestro"),
			createTables("us-east-1", dbEndpoint),
			createGRPCClient(maestroGPRCBaseURL),
			createHttpClient(),
		)
		if os.Getenv("CLEAN_ENV") == "true" {
			testenv.Finish(
				deleteHttpClient(),
				deleteGRPCClient(),
				uninstallComponent("../manifests/maestro"),
				uninstallComponent("../manifests/dynamodb"),
				uninstallComponent("../manifests/work-agent"),
				uninstallComponent("../manifests/mqtt-broker"),
			)
		} else {
			testenv.Finish(
				deleteHttpClient(),
				deleteGRPCClient(),
			)
		}
	} else {
		testenv = env.NewWithConfig(cfg)
		kindClusterName := envconf.RandomName("maestro-e2e", 16)

		testenv.Setup(
			envfuncs.CreateClusterWithConfig(kind.NewProvider(), kindClusterName, "kind-config.yaml", kind.WithImage("kindest/node:v1.27.1")),
			installComponent("../manifests/mqtt-broker"),
			installComponent("../manifests/work-agent"),
			installComponent("../manifests/dynamodb"),
			installComponent("../manifests/maestro"),
			createTables("us-east-1", dbEndpoint),
			createGRPCClient(maestroGPRCBaseURL),
			createHttpClient(),
		)

		if os.Getenv("CLEAN_ENV") == "true" {
			testenv.Finish(
				deleteHttpClient(),
				deleteGRPCClient(),
				uninstallComponent("../manifests/maestro"),
				uninstallComponent("../manifests/dynamodb"),
				uninstallComponent("../manifests/work-agent"),
				uninstallComponent("../manifests/mqtt-broker"),
				envfuncs.DestroyCluster(kindClusterName),
			)
		} else {
			testenv.Finish(
				deleteHttpClient(),
				deleteGRPCClient(),
			)
		}
	}

	os.Exit(testenv.Run(m))
}

func installComponent(kustomizationPath string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		mqttManifests, err := kustomize.Render(kustomize.Options{
			KustomizationPath: kustomizationPath,
		})
		if err != nil {
			fmt.Printf("Error rendering manifests: %v\n", err)
			return ctx, err
		}

		objects, err := kustomize.ToObjects(mqttManifests)
		if err != nil {
			fmt.Printf("Error converting manifests to objects: %v\n", err)
			return ctx, err
		}

		for _, obj := range objects {
			if err := cfg.Client().Resources().Create(ctx, obj); err != nil {
				fmt.Printf("Error creating object: %v\n", err)
				return ctx, err
			}
		}

		return ctx, nil
	}
}

func uninstallComponent(kustomizationPath string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		mqttManifests, err := kustomize.Render(kustomize.Options{
			KustomizationPath: kustomizationPath,
		})
		if err != nil {
			fmt.Printf("Error rendering manifests: %v\n", err)
			return ctx, err
		}

		objects, err := kustomize.ToObjects(mqttManifests)
		if err != nil {
			fmt.Printf("Error converting manifests to objects: %v\n", err)
			return ctx, err
		}

		for _, obj := range objects {
			if err := cfg.Client().Resources().Delete(ctx, obj); err != nil {
				fmt.Printf("Error deleting object: %v\n", err)
				return ctx, err
			}
		}

		return ctx, nil
	}
}

func createTables(region, endpoint string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		client, err := cfg.NewClient()
		if err != nil {
			return ctx, err
		}
		dynamodbDep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dynamodb", Namespace: "dynamodb"},
		}
		// wait for the deployment to become at least 100%
		err = wait.For(conditions.New(client.Resources()).ResourceMatch(dynamodbDep, func(object k8s.Object) bool {
			d := object.(*appsv1.Deployment)
			return float64(d.Status.ReadyReplicas)/float64(*d.Spec.Replicas) >= 1.0
		}), wait.WithTimeout(time.Minute*2))
		if err != nil {
			return ctx, err
		}

		fmt.Printf("deployment availability: %.2f%%\n", float64(dynamodbDep.Status.ReadyReplicas)/float64(*dynamodbDep.Spec.Replicas)*100)

		dynamodbConfig, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithEndpointResolver(aws.EndpointResolverFunc(
				func(service, region string) (aws.Endpoint, error) {
					return aws.Endpoint{URL: endpoint}, nil
				})),
			// config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			// 	Value: aws.Credentials{
			// 		AccessKeyID:     accessKeyID,
			// 		SecretAccessKey: secretAccessKey,
			// 	},
			// }),
		)

		if err != nil {
			fmt.Printf("Error loading AWS DynamoDB config: %v\n", err)
			return ctx, err
		}

		dynamodbClient, err := dynamodb.NewFromConfig(dynamodbConfig), nil
		if err != nil {
			fmt.Printf("Error creating AWS DynamoDB client: %v\n", err)
			return ctx, err
		}

		tableName := "Consumers"
		tableInput := &dynamodb.CreateTableInput{
			AttributeDefinitions: []types.AttributeDefinition{{
				AttributeName: aws.String("Id"),
				AttributeType: types.ScalarAttributeTypeS,
			}},
			KeySchema: []types.KeySchemaElement{{
				AttributeName: aws.String("Id"),
				KeyType:       types.KeyTypeHash,
			}},
			TableName: aws.String(tableName),
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(5),
				WriteCapacityUnits: aws.Int64(5),
			},
		}

		err = wait.For(func(ctx context.Context) (done bool, err error) {
			_, err = dynamodbClient.CreateTable(ctx, tableInput)
			if err != nil {
				done = false
			} else {
				done = true
			}
			return
		}, wait.WithInterval(time.Second*20), wait.WithTimeout(time.Minute*2))
		if err != nil {
			fmt.Printf("Error creating table(%v): %v\n", tableName, err)
			return ctx, err
		}

		waiter := dynamodb.NewTableExistsWaiter(dynamodbClient)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName}, 5*time.Minute)
		if err != nil {
			fmt.Printf("Wait for table exists failed: %v\n", err)
			return ctx, err
		}

		tableName = "Resources"
		tableInput = &dynamodb.CreateTableInput{
			AttributeDefinitions: []types.AttributeDefinition{{
				AttributeName: aws.String("Id"),
				AttributeType: types.ScalarAttributeTypeS,
			}},
			KeySchema: []types.KeySchemaElement{{
				AttributeName: aws.String("Id"),
				KeyType:       types.KeyTypeHash,
			}},
			TableName: aws.String(tableName),
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(5),
				WriteCapacityUnits: aws.Int64(5),
			},
		}
		err = wait.For(func(ctx context.Context) (done bool, err error) {
			_, err = dynamodbClient.CreateTable(ctx, tableInput)
			if err != nil {
				done = false
			} else {
				done = true
			}
			return
		}, wait.WithInterval(time.Second*20), wait.WithTimeout(time.Minute*2))
		if err != nil {
			fmt.Printf("Error creating table(%v): %v\n", tableName, err)
			return ctx, err
		}

		waiter = dynamodb.NewTableExistsWaiter(dynamodbClient)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName}, 5*time.Minute)
		if err != nil {
			fmt.Printf("Wait for table exists failed: %v\n", err)
			return ctx, err
		}

		return ctx, nil
	}
}

func createHttpClient() env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

		client := &http.Client{
			Transport: transport,
		}

		return context.WithValue(ctx, "http-client", client), nil
	}
}

func deleteHttpClient() env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		httpClientValue := ctx.Value("http-client")
		if httpClientValue == nil {
			return ctx, fmt.Errorf("delete http client func: context http client is nil")
		}

		httpClient, ok := httpClientValue.(*http.Client)
		if !ok {
			return ctx, fmt.Errorf("delete http client func: unexpected type for http client value")
		}

		httpClient.CloseIdleConnections()
		return ctx, nil
	}
}

func createGRPCClient(endpoint string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Printf("Error initializing GRPC connection: %v\n", err)
			return ctx, err
		}

		return context.WithValue(ctx, "grpc-connction", conn), nil
	}
}

func deleteGRPCClient() env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		connValue := ctx.Value("grpc-connction")
		if connValue == nil {
			return ctx, fmt.Errorf("delete grpc client func: context grpc connection is nil")
		}

		conn, ok := connValue.(*grpc.ClientConn)
		if !ok {
			return ctx, fmt.Errorf("delete grpc client func: unexpected type for grpc connection value")
		}

		conn.Close()
		return ctx, nil
	}
}
