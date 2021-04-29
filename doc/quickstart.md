# Quickstart Installation Guide

- [Quickstart Installation Guide](#quickstart-installation-guide)
  - [kubeadm](#kubeadm)
  - [kubespray](#kubespray)

## kubeadm

Install the [docker](https://docs.docker.com/engine/install/ubuntu/) in the Kubernetes cluster node.
Follow the steps in [create cluster kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create kubernetes cluster in master
In the master node run the `kubeadm init` as below. The ovn4nfv uses pod network cidr `10.233.64.0/18`

```bash
kubeadm init --kubernetes-version=1.19.0 --pod-network-cidr=10.233.64.0/18 --apiserver-advertise-address=<master_eth0_ip_address>
```

Ensure the master node taint for no schedule is removed and labelled with `ovn4nfv-k8s-plugin=ovn-control-plane`

```bash
nodename=$(kubectl get node -o jsonpath='{.items[0].metadata.name}')
kubectl taint node $nodename node-role.kubernetes.io/master:NoSchedule-
kubectl label --overwrite node $nodename ovn4nfv-k8s-plugin=ovn-control-plane
```

Deploy the ovn4nfv Pod network to the cluster.

```bash
kubectl apply -f deploy/ovn-daemonset.yaml
kubectl apply -f deploy/ovn4nfv-k8s-plugin.yaml
```

Join worker node by running the `kubeadm join` on each node as root as mentioned in [create cluster kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/)

## kubespray

Kubespray support the ovn4nfv as the network plugin- please follow the steps in [kubernetes-sigs/kubespray//docs/ovn4nfv.md](https://github.com/kubernetes-sigs/kubespray/blob/master/docs/ovn4nfv.md)
