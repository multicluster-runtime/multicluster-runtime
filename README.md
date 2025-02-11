# multicluster-runtime

<img src="https://github.com/user-attachments/assets/bfb80971-1ec1-4922-b201-b23f522f829d" width="150">
<br/>

Multi cluster controllers with controller-runtime
- **no fork, no go mod replace**: unmodified [upstream controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) under the hood.
- **universal**: cluster-api. kcp. BYO. Cluster providers make the controller-runtime multi-cluster aware.
- **seamless**: single-cluster and multi-cluster mode without code changes to the reconcilers.
