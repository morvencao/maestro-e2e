package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

func TestMain(m *testing.M) {
	testenv = env.New()
	if os.Getenv("REAL_CLUSTER") == "true" {
		path := conf.ResolveKubeConfigFile()
		cfg := envconf.NewWithKubeConfig(path)
		testenv = env.NewWithConfig(cfg)

		testenv.Setup(
			installComponent("../manifests/mqtt-broker"),
			installComponent("../manifests/work-agent"),
			installComponent("../manifests/dynamodb"),
			installComponent("../manifests/maestro"),
			createTables,
		)
		if os.Getenv("CLEAN_ENV") == "true" {
			testenv.Finish(
				uninstallComponent("../manifests/maestro"),
				uninstallComponent("../manifests/dynamodb"),
				uninstallComponent("../manifests/work-agent"),
				uninstallComponent("../manifests/mqtt-broker"),
			)
		}
	} else {
		kindClusterName := envconf.RandomName("kind-with-config", 16)

		testenv.Setup(
			envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
			envfuncs.CreateClusterWithConfig(kind.NewProvider(), kindClusterName, "kind-config.yaml", kind.WithImage("kindest/node:v1.27.1")),
			installComponent("../manifests/mqtt-broker"),
			installComponent("../manifests/work-agent"),
			installComponent("../manifests/dynamodb"),
			installComponent("../manifests/maestro"),
			createTables,
		)

		if os.Getenv("CLEAN_ENV") == "true" {
			testenv.Finish(
				uninstallComponent("../manifests/maestro"),
				uninstallComponent("../manifests/dynamodb"),
				uninstallComponent("../manifests/work-agent"),
				uninstallComponent("../manifests/mqtt-broker"),
				envfuncs.DestroyCluster(kindClusterName),
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

func createTables(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	client, err := cfg.NewClient()
	if err != nil {
		return ctx, err
	}
	dynamodbDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dynamodb", Namespace: "dynamodb"},
	}
	// wait for the deployment to become at least 50%
	err = wait.For(conditions.New(client.Resources()).ResourceMatch(dynamodbDep, func(object k8s.Object) bool {
		d := object.(*appsv1.Deployment)
		return float64(d.Status.ReadyReplicas)/float64(*d.Spec.Replicas) >= 0.50
	}), wait.WithTimeout(time.Minute*2))
	if err != nil {
		return ctx, err
	}

	fmt.Printf("deployment availability: %.2f%%\n", float64(dynamodbDep.Status.ReadyReplicas)/float64(*dynamodbDep.Spec.Replicas)*100)

	dynamodbConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithEndpointResolver(aws.EndpointResolverFunc(
			func(service, region string) (aws.Endpoint, error) {
				return aws.Endpoint{URL: fmt.Sprintf("http://127.0.0.1:31310")}, nil
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
	table, err := dynamodbClient.CreateTable(ctx, &dynamodb.CreateTableInput{
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
	})
	if err != nil {
		fmt.Printf("Error creating table(%v): %v\n", tableName, err)
		return ctx, err
	} else {
		waiter := dynamodb.NewTableExistsWaiter(dynamodbClient)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName}, 5*time.Minute)
		if err != nil {
			fmt.Printf("Wait for table exists failed: %v\n", err)
			return ctx, err
		}
	}

	fmt.Printf("Table(%v) created successfully\n", table.TableDescription.TableName)

	tableName = "Resources"
	table, err = dynamodbClient.CreateTable(ctx, &dynamodb.CreateTableInput{
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
	})
	if err != nil {
		fmt.Printf("Error creating table(%v): %v\n", tableName, err)
		return ctx, err
	} else {
		waiter := dynamodb.NewTableExistsWaiter(dynamodbClient)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName}, 5*time.Minute)
		if err != nil {
			fmt.Printf("Wait for table exists failed: %v\n", err)
			return ctx, err
		}
	}

	fmt.Printf("Table(%v) created successfully\n", table.TableDescription.TableName)

	return ctx, nil
}
