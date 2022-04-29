![nodus_logo](https://github.com/akraino-edge-stack/icn-ovn4nfv-k8s-network-controller/blob/master/images/logo/nodus_logo.png)
# Nodus - Network Controller
Nodus is Network controller in Kubernetes that address multiple network use case as below
- Multi ovn network support
- Multi-interface ovn support
- Multi-IP address support
- Dynamic creation of virtual networks
- Route management across virtual networks and external networks
- Service Function chaining(SFC) support in Kubernetes
- OVN ACL based Network Policy
- Secure Nodus and OVN Network Traffic([WIP](https://gerrit.akraino.org/r/c/icn/nodus/+/4838))
- SRIOV Overlay networking (WIP)
- OVN load balancer (WIP)

Nodus is latin word for "knot". Nodus converge multiple kubernetes networking use cases in a single network controller.

## How it works

Nodus consist of 4 major components
- OVN control plane
- OVN controller
- Network Function Network(NFN) k8s operator/controller
- Network Function Network(NFN) agent

OVN control plane and OVN controller take care of OVN configuration and installation in each node in Kubernetes. NFN operator runs in the Kubernetes master and NFN agent run as a daemonset in each node.

### Nodus architecture blocks
![ovn4nfv k8s arc block](./images/ovn4nfv-k8s-arch-block.png)

#### NFN Operator
* Exposes virtual, provider, chaining CRDs to external world
* Programs OVN to create L2 switches
* Watches for PODs being coming up
 * Assigns IP addresses for every network of the deployment
 * Looks for replicas and auto create routes for chaining to work
 * Create LBs for distributing the load across CNF replicas
#### NFN Agent
* Performs CNI operations.
* Configures VLAN and Routes in Linux kernel (in case of routes, it could do it in both root and network namespaces)
* Communicates with OVSDB to inform of provider interfaces. (creates ovs bridge and creates external-ids:ovn-bridge-mappings)

### Networks traffice between pods
![ovn4nfv network traffic](./images/ovn4nfv-network-traffic.png)

ovn4nfv-default-nw is the default logic switch create for the default networking in kubernetes pod network for cidr 10.233.64.0/18. Both node and pod in the kubernetes cluster share the same ipam information.

### Service Function Chaining Demo
![sfc-with-sdewan](./images/sfc-with-sdewan.png)

In general production env, we have multiple Network function such as SLB, NGFW and SDWAN CNFs.

There are general 3 sfc flows are there:
* Packets from the pod to reach internet: Ingress (L7 LB) -> SLB -> NGFW -> SDWAN CNF -> External router -> Internet
* Packets from the pod to internal server in the corp network: Ingress (L7 LB) -> SLB -> M3 server
* Packets from the internal server M3 to reach internet: M3 -> SLB -> NGFW -> SDWAN CNF -> External router -> Internet

Nodus SFC currently support all 3 flows.

#### Demos

- [Dynamic Network - SFC](./demo/calico-nodus-secondary-sfc-setup-II/README.md)
- [Secondary Network - SFC](./demo/calico-nodus-secondary-sfc-setup/README.md)
- [Primary Network - SFC](./demo/nodus-primary-sfc-setup/README.md)

# Quickstart Installation Guide
### kubeadm

Install the [docker](https://docs.docker.com/engine/install/ubuntu/) in the Kubernetes cluster node.
Follow the steps in [create cluster kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create kubernetes cluster in master
In the master node run the `kubeadm init` as below. The Nodus uses pod network cidr `10.210.0.0/16`
```
    $ kubeadm init --kubernetes-version=1.23.3 --pod-network-cidr=10.210.0.0/16 --apiserver-advertise-address=<master_eth0_ip_address>
```
Ensure the master node taint for no schedule is removed and labelled with `ovn4nfv-k8s-plugin=ovn-control-plane`
```
nodename=$(kubectl get node -o jsonpath='{.items[0].metadata.name}')
kubectl taint node $nodename node-role.kubernetes.io/master:NoSchedule-
kubectl label --overwrite node $nodename ovn4nfv-k8s-plugin=ovn-control-plane
```

[Kustomize](https://kustomize.io/) and deploy [cert-manager](https://cert-manager.io/):
```
$ curl -Ls https://github.com/cert-manager/cert-manager/releases/download/v1.8.0/cert-manager.yaml -o deploy/cert-manager/cert-manager.yaml && kubectl apply -k deploy/cert-manager/
```

Deploy the Nodus Pod network to the cluster.
```
    $ kubectl apply -f deploy/ovn-daemonset.yaml
    $ kubectl apply -f deploy/ovn4nfv-k8s-plugin.yaml
```

Ensure the configmap `ovn-controller-network` data `OVN_SUBNET` matches the pod network cidr as well in `deploy/ovn4nfv-k8s-plugin.yaml`
Join worker node by running the `kubeadm join` on each node as root as mentioned in [create cluster kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/)

### kubespray

Kubespray support the Nodus as the network plugin- please follow the steps in [kubernetes-sigs/kubespray/docs/ovn4nfv.md](https://github.com/kubernetes-sigs/kubespray/blob/master/docs/ovn4nfv.md)

### Nodus K8s security requirements
#### ETCD
The etcd store data such as cluster state and k8s secrets. The best approach is to set up a firewall between the Kubernetes API server in the control plane node and etcd in a different node, the access to the etcd is limited by the firewall for the API server in the control plane only. The user must always ensure the mutual auth via TLS client certificate. More information to setup can be found in the [etcd documentation](https://etcd.io/docs/v3.2/op-guide/security/#basic-setup)
####  Encryption of secret data at rest
By default k8s secret data stored in the etcd are not encrypted. The user must use the KMS encryption provider for strong encryption - [link](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/). The user should not use other encryption provider mechanisms such as secretbox(XSalsa20 and Poly1305 encryption), aesgcm(AES-GCM with random nonce), aescbc(AES-CBC with PKCS#7 padding) as they are show vulnerability/not recommended in the [Kubernetes documentations](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/#providers)
#### File in Monitoring for Nodus logs
Add `/var/log/openvswitch/ovn4k8s.log` in the audit.rules to monitor the log files and ensure the logs are not tampered - [link](https://docs.rapid7.com/insightidr/fim-for-linux/)

## Comprehensive Documentation

- [How to use](doc/how-to-use.md)
- [Configuration](doc/configuration.md)
- [Development](doc/development.md)
- [Validation & testcase](https://wiki.akraino.org/display/AK/ICN+R6+Test+Document#ICNR6TestDocument-NodusValidationandtestcaseresults)
- [Akraino ICN Recommended Operating system security tools](https://wiki.akraino.org/display/AK/ICN+R6+Test+Document#ICNR6TestDocument-BluValTesting)

## Contact Us

For any questions about Nodus k8s , feel free to ask a question in #general in the [ICN slack](https://akraino-icn-admin.herokuapp.com/), or open up a https://jira.akraino.org/projects/ICN/issues.
