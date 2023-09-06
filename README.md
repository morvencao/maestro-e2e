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
kind create cluster --config kind-config.yaml``
```

Then, run the testing with the following command:

```bash
REAL_CLUSTER=true go test -v ./e2e/...
```

2. Creating new KinD cluster for testing

```bash
go test -v ./e2e/...
```

Note: By default, the cluster and testing resources will not be deleted after the testing. If you want to delete them after the testing, you can set the environment variable `CLEAN_ENV` to `true`:

```bash
CLEAN_ENV=true go test -v ./e2e/...
```
