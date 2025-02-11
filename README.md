# multicluster-runtime

<img src="https://github.com/user-attachments/assets/452f680b-c635-4c67-9099-e35a08ca5e02" width="200">
<br/>

Multi cluster controllers with controller-runtime
- **no fork, no go mod replace**: extension to the unmodified [upstream controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
- **universal**: cluster-api. kcp. BYO. Cluster providers make the controller-runtime multi-cluster aware.
- **seamless**: add multi-cluster support without compromising on single-cluster. Run in either mode without code changes to the reconcilers. 

## Uniform Reconcilers

Run the same reconciler against many clusters:
- The reconciler reads from cluster A and writes to cluster A.
- The reconciler reads from cluster B and writes to cluster B.
- The reconciler reads from cluster C and writes to cluster C.

This is the most simple case. Many existing reconcilers can easily adapted to work like this without major code changes. The resulting controllers will work in the multi-cluster setting, but also in the classical single-cluster setup, all in the same code base.

![multi-cluster topologies simple](https://github.com/user-attachments/assets/a57a0d05-4ca0-42a6-b064-90b728247a24)

## Multi-Cluster-aware Reconcilers

Run reconcilers that listen to some cluster(s) and operate other clusters.

![multi-cluster topologies cross](https://github.com/user-attachments/assets/68f3813d-58da-46ae-9f40-52882348ae02)
