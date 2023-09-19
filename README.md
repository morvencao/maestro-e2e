# maestro-e2e

e2e testing for [maestro project](https://github.com/kube-orchestra/maestro)

## Prerequisites

- [Golang](https://golang.org/)
- [KinD](https://kind.sigs.k8s.io/)
- [Docker](https://www.docker.com/) or [Podman](https://podman.io/)

## Get Started

You have two options to run the e2e testing:

1. Using existing Kubernetes cluster

For example, you can create a KinD cluster using configuration file `kind-config.yaml`:

```bash
kind create cluster --config e2e/kind-config.yaml
```

Then, run the testing with the following command:

```bash
REAL_CLUSTER=true go test ./e2e
```

2. Creating new KinD cluster for testing

```bash
go test ./e2e
```

Note: By default, the cluster and testing resources will not be deleted after the testing. If you want to delete them after the testing, you can set the environment variable `CLEAN_ENV` to `true`:

```bash
CLEAN_ENV=true go test ./e2e
```

3. You can easily skip specific tests based on labels using the following command:

```bash
go test ./e2e -args --skip-labels="type=rest"
```

To skip GRPC tests, use the label `type=grpc`:

```bash
go test ./e2e -args --skip-labels="type=grpc"
```

To skip tests for the manifest (cloudevent) API, use the label `res=manifest`:

```bash
go test ./e2e -args --skip-labels="res=manifest"
```

_Note:_ you can't skip consumer tests, as they are required for the other tests to run.

By utilizing these labels, you can easily customize your testing suite to exclude specific test types as needed.

## Manual Testing

To streamline the process of setting up the testing environment, you can simply follow these steps.

1. Clone the repository:

```bash
git clone git@github.com:morvencao/maestro-e2e.git
cd maestro-e2e
```

2. Set up the environment by deploying Maestro and Work-Agent into a KinD cluster:

```bash
go test -v ./env-setup
```

The output should resemble this:

```bash
# go test -v ./env-setup
database deployment availability: 100.00%
database table created: Consumers
database table created: Resources
maestro deployment availability: 100.00%
consumer created: f7384ef8-37bf-4cb2-8682-9b2f00b6f457
consumer retrieved: f7384ef8-37bf-4cb2-8682-9b2f00b6f457
work-agent deployment availability: 100.00%
testing: warning: no tests to run
PASS
ok  	github.com/morvencao/maestro-e2e/env-setup	94.504s [no tests to run]
```

3. You can then access the Maestro API server REST endpoint at http://localhost:31330 and the GRPC endpoint at `localhost:31320`. The consumer ID is obtained from the output of the previous step.

```bash
CONSUMER_ID="f7384ef8-37bf-4cb2-8682-9b2f00b6f457"
```

4. To create, retrieve, and update a resource for testing, utilize the following commands:

```bash
# create resource
curl -X POST localhost:31330/v1/consumers/$CONSUMER_ID/resources -H "Content-Type: application/json" --data-binary @examples/deployment.json
{
  "id": "11ddef7f-2816-4779-a25a-496660005cff",
  "consumerId": "f7384ef8-37bf-4cb2-8682-9b2f00b6f457",
  "generationId": "1",
  "object": {
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    ...
  },
  "status": null
}

# retrieve resource
RESOURCE_ID="11ddef7f-2816-4779-a25a-496660005cff"
curl localhost:31330/v1/resources/$RESOURCE_ID

# update resource
curl -X PUT localhost:31330/v1/resources/$RESOURCE_ID -H "Content-Type: application/json" --data-binary @examples/deployment.v2.json
```
