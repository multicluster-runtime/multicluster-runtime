# multicluster-runtime

<img src="./contrib/logo/logo.png" width="300"/>

## Multi cluster controllers with controller-runtime

- **no fork, no go mod replace**: extension to the unmodified [upstream controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
- **universal**: cluster-api. kcp. BYO. Cluster providers make the controller-runtime multi-cluster aware.
- **seamless**: add multi-cluster support without compromising on single-cluster. Run in either mode without code changes to the reconcilers. 

## Uniform Reconcilers

Run the same reconciler against many clusters:
- The reconciler reads from cluster A and writes to cluster A.
- The reconciler reads from cluster B and writes to cluster B.
- The reconciler reads from cluster C and writes to cluster C.

This is the most simple case. Many existing reconcilers can easily adapted to work like this without major code changes. The resulting controllers will work in the multi-cluster setting, but also in the classical single-cluster setup, all in the same code base.

![multi-cluster topologies uniform](https://github.com/user-attachments/assets/b91a3aac-6a1c-481e-8961-2f25605aeffe)

## Multi-Cluster-aware Reconcilers

Run reconcilers that listen to some cluster(s) and operate other clusters.
![multi-cluster topologies multi](https://github.com/user-attachments/assets/d7e37c39-66e3-4912-89ac-5441f0ad5669)

## Principles

1. multicluster-runtime is a friendly ❤️ extension of controller-runtime.
2. multicluster-runtime loves ❤️ contributions.
3. multicluster-runtime is following controller-runtime releases.
4. multicluster-runtime is developed as if it was part of controller-runtime (quality standards, naming, style).
5. multicluster-runtime is provider agnostic, but may contain providers with its own go.mod files and dedicated OWNERS files.
