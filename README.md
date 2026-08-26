# Genoa

A small kubernetes job that runs in front of a Rotational deployment to ensure the environment is setup.

## Local Development

In order to develop and test Genoa, I'm using a [kind](https://kind.sigs.k8s.io/) kubernetes cluster since docker compose will not help us with kubernetes specific operations. Ensure you have `kind`, `helm`, and `helmfile` installed using `brew`:

```
$ brew install kind helm helmfile
```

You can then create a local cluster using the development config:

```
$ kind create cluster --config dev/kind/config.yaml
$ kind get clusters
rotational-local-1
```

Ensure you are connected to the kind cluster:

```
$ kubectl config current-context
kind-rotational-local-1
```

Install postgresql with the CNPG operator using helmfile:

```
$ cd dev
$ helmfile apply
```

You should now be able to connect to the database once the postgres pod is ready:

```
$ kubectl get pods -n cnpg
NAME                                   READY   STATUS    RESTARTS   AGE
cnpg-cloudnative-pg-5bcdbd467f-k9d6z   1/1     Running   0          81s
postgres-1                             1/1     Running   0          44s
```

```
$ PGPASSWORD=theeaglefliesatmidnight psql -d postgres -U postgres -p 7654 -h localhost
```

### Cleaning up

To delete the kind cluster run the following:

```
$ kind delete cluster --name rotational-local-1
$ rm -rf dev/kind/data
```

This will remove the cluster and any local PVCs that were created.

## About

A [genoa](https://en.wikipedia.org/wiki/Genoa_(sail)) is a large type of [jib](https://en.wikipedia.org/wiki/Jib) or [staysail](https://en.wikipedia.org/wiki/Staysail) that extends past the mast and so overlaps the mainsail when vieweed from the side. Often also called a "genny", it increases the speed of the craft in light to moderate winds by running out in front of the ship.