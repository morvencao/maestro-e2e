# maestro-e2e

e2e testing for [maestro project](https://github.com/kube-orchestra/maestro)

## Prerequisites

- Golang
- KinD
- Docker

## Get Started

You have two options to run the e2e testing:

1. Using existing Kubernetes cluster

For example, you can create a KinD cluster using configuration file `kind-config.yaml`:

```bash
kind create cluster --config e2e/kind-config.yaml``
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
