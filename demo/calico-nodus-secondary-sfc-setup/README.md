# Service Function Chaining(SFC) - setup

## Summary

This project offers a means for deploying a Kubernetes cluster
that satisfies the requirements of Nodus sfc setup

## Virtual Machines

This project uses [Vagrant tool][2] for provisioning Virtual Machines
automatically. The [setup](setup.sh) bash script contains the
Linux instructions to install dependencies and plugins required for
its usage. This script supports two Virtualization technologies
(Libvirt and VirtualBox).

```
    $ sudo ./setup.sh -p libvirt
```
There is a `default.yml` in the `./config` directory which creates multiple vm.

Once Vagrant is installed, it's possible to provision a vm using
the following instructions:
```
    $ vagrant up
```
In-depth documentation and use cases of various Vagrant commands [Vagrant commands][3]
is available on the Vagrant site.

## Deployment

### How to create K8s cluster?

Install the [docker](https://docs.docker.com/engine/install/ubuntu/) in the master, minion01 and minion02 vm.
Follow the steps in [create cluster kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create kubernetes cluster in master

### kubeadm
In the master node run the `kubeadm init` as below. The calico uses pod network cidr `10.210.0.0/16`
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

Deploy the Calico and Multus CNI in the kubeadm master
```
     $ kubectl apply -f deploy/calico.yaml
     $ kubectl apply -f deploy/multus-daemonset.yaml
```
One of major change, we required to do for calico is to enable ip forwarding in the container network namespace.
This is enabled by macro `allow_ip_forwarding` to `true` in the calico cni configuration file.

There will be multiple conf files, we have to make sure Multus file is in the Lexicographic order.
Kubernetes kubelet is designed to pick the config file in the lexicograpchic order.

In this example, we are using pod CIDR as `10.210.0.0/16`. The Calico will automatically detect the CIDR based on the running configuration.
Since calico network going to the primary network in our case, nodus subnet should be a different network. Make sure you change the `OVN_SUBNET` and `OVN_GATEWAYIP` in `deploy/ovn4nfv-k8s-plugin.yaml`
In this example, we customize the ovn network as follows.
```
data:
  OVN_SUBNET: "10.154.141.0/18"
  OVN_GATEWAYIP: "10.154.141.1/18"
```
Deploy the Nodus Pod network to the cluster.
```
    $ kubectl apply -f deploy/ovn-daemonset.yaml
    $ kubectl apply -f deploy/ovn4nfv-k8s-plugin.yaml
```

Join minion01 and minion02 by running the `kubeadm join` on each node as root as mentioned in [create cluster kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/)

### notes
Calico by default could pick the interfaces without internet access. If user required to access internet access.
Run the following command to pick the interface that has internet access.

```
   $ kubectl set env daemonset/calico-node -n kube-system IP_AUTODETECTION_METHOD=can-reach=www.google.com
```

### TM1 server

ssh into the TM1 vm and run the following command to attach TM1 to the left provider network.
```
    $ ip addr flush dev eth1
    $ ip link add link eth1 name eth1.100 type vlan id 100
    $ ip link set dev eth1.100 up
    $ ip addr add 172.30.10.101/24 dev eth1.100
    $ ip route del default
    $ ip route add default via 172.30.10.3
```
### TM2 server

ssh into the TM2 vm and run the following command to attach TM2 to the right provider network.
```
    $ ip addr flush dev eth1
    $ ip link add link eth1 name eth1.200 type vlan id 200
    $ ip link set dev eth1.200 up
    $ ip addr add 172.30.20.2/24 dev eth1.200
```
Run the following commands to create virtual router
```
   $ ip route add 172.30.10.0/24 via 172.30.20.3
   $ ip route add 172.30.33.0/24 via 172.30.20.3
   $ ip route add 172.30.44.0/24 via 172.30.20.3
   $ ip route add 172.30.11.0/24 via 172.30.20.3
   $ ip route add 172.30.22.0/24 via 172.30.20.3
```
```
   $ echo 1 > /proc/sys/net/ipv4/ip_forward
   $ /sbin/iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
   $ iptables -A FORWARD -i eth1 -o eth0 -j ACCEPT
   $ iptables -A FORWARD -i eth1.200 -o eth0 -j ACCEPT
```
## Demo setup

The setup show the SFC is connected to two network. One virtual and provider network.
![sfc-virtual-and-provider-network-setup](../../images/sfc-virtual-and-provider-network-setup.png)

let create the demo setup
```
   $ kubectl apply -f example/multus-net-attach-def-cr.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sfc-virtual-network.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/slb-multiple-network.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/ngfw.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sdewan-multiple-network.yaml
```
- The above commends created the multiple networks - virtual-network-1, provider-network-1, dynamic-network-1
dynamic-network-2, virtual-network-2 and provider-network-2. The corresponding vlan tagging is created in the nodes
- Dummy VFs application are deployed in this case are Smart Load balancer,Next Generation Firewall and Software Defined
Edge WAN. This could be replaced by the actual VFs application.

Next steps to deploy Pods and deploy the SFCs

```
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/namespace-right.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/namespace-left.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/nginx-left-deployment.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/nginx-right-deployment.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sfc-with-virtual-and-provider-network.yaml
```
Let trace the packet flow in the sfc for the internal and external traffic throug sfc

```
   $ kubectl get pods -A -o wide
NAMESPACE     NAME                                       READY   STATUS    RESTARTS   AGE     IP               NODE       NOMINATED NODE   READINESS GATES
default       ngfw-66b7d958c9-sgtj8                      1/1     Running   0          3m2s    10.210.50.75     minion01   <none>           <none>
default       sdewan-8b8867c49-jpg78                     1/1     Running   0          2m52s   10.210.50.76     minion01   <none>           <none>
default       slb-7bc67f7866-894sk                       1/1     Running   0          3m10s   10.210.50.74     minion01   <none>           <none>
kube-system   calico-kube-controllers-66d6894896-mmmrk   1/1     Running   0          5h10m   10.210.50.65     minion01   <none>           <none>
kube-system   calico-node-krkgr                          1/1     Running   0          5h6m    192.168.121.28   minion01   <none>           <none>
kube-system   calico-node-mpx2s                          1/1     Running   0          5h6m    192.168.121.13   master     <none>           <none>
kube-system   coredns-64897985d-9t2wf                    1/1     Running   0          5h58m   10.210.50.67     minion01   <none>           <none>
kube-system   coredns-64897985d-z2sjk                    1/1     Running   0          5h58m   10.210.50.66     minion01   <none>           <none>
kube-system   etcd-master                                1/1     Running   0          5h58m   192.168.121.13   master     <none>           <none>
kube-system   kube-apiserver-master                      1/1     Running   0          5h58m   192.168.121.13   master     <none>           <none>
kube-system   kube-controller-manager-master             1/1     Running   0          5h58m   192.168.121.13   master     <none>           <none>
kube-system   kube-multus-ds-amd64-bj5j7                 1/1     Running   0          5h9m    192.168.121.28   minion01   <none>           <none>
kube-system   kube-multus-ds-amd64-lbt98                 1/1     Running   0          5h9m    192.168.121.13   master     <none>           <none>
kube-system   kube-proxy-pb4nj                           1/1     Running   0          5h58m   192.168.121.13   master     <none>           <none>
kube-system   kube-proxy-vdj5g                           1/1     Running   0          5h57m   192.168.121.28   minion01   <none>           <none>
kube-system   kube-scheduler-master                      1/1     Running   0          5h58m   192.168.121.13   master     <none>           <none>
kube-system   nfn-agent-879w7                            1/1     Running   0          7m57s   192.168.121.13   master     <none>           <none>
kube-system   nfn-agent-hwrrj                            1/1     Running   0          7m57s   192.168.121.28   minion01   <none>           <none>
kube-system   nfn-operator-7c465f466b-4kqs2              1/1     Running   0          7m57s   192.168.121.13   master     <none>           <none>
kube-system   ovn-control-plane-7dd9ff64c8-9hmwn         1/1     Running   0          5h6m    192.168.121.13   master     <none>           <none>
kube-system   ovn-controller-c94br                       1/1     Running   0          5h6m    192.168.121.28   minion01   <none>           <none>
kube-system   ovn-controller-pf8hb                       1/1     Running   0          5h6m    192.168.121.13   master     <none>           <none>
kube-system   ovn4nfv-cni-pm4wd                          1/1     Running   0          7m57s   192.168.121.13   master     <none>           <none>
kube-system   ovn4nfv-cni-xlbs4                          1/1     Running   0          7m57s   192.168.121.28   minion01   <none>           <none>
sfc-head      nginx-left-deployment-7476fb75fc-g8brt     1/1     Running   0          73s     10.210.50.78     minion01   <none>           <none>
sfc-head      nginx-left-deployment-7476fb75fc-h8w6s     1/1     Running   0          73s     10.210.50.77     minion01   <none>           <none>
sfc-head      nginx-left-deployment-7476fb75fc-qz8nz     1/1     Running   0          73s     10.210.219.68    master     <none>           <none>
sfc-tail      nginx-right-deployment-965b96d57-65bvx     1/1     Running   0          64s     10.210.50.79     minion01   <none>           <none>
sfc-tail      nginx-right-deployment-965b96d57-cvrpt     1/1     Running   0          64s     10.210.219.69    master     <none>           <none>
sfc-tail      nginx-right-deployment-965b96d57-ff7pg     1/1     Running   0          64s     10.210.50.80     minion01   <none>           <none>
```
### Flow I
Let ping from the left pod to right pod and left pod to internet
```
$ kubectl exec -it nginx-right-deployment-965b96d57-65bvx -n sfc-tail -- ifconfig
eth0      Link encap:Ethernet  HWaddr EE:97:C9:3A:85:C3
          inet addr:10.210.50.79  Bcast:10.210.50.79  Mask:255.255.255.255
          UP BROADCAST RUNNING MULTICAST  MTU:1440  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:0
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

lo        Link encap:Local Loopback
          inet addr:127.0.0.1  Mask:255.0.0.0
          UP LOOPBACK RUNNING  MTU:65536  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

sn0       Link encap:Ethernet  HWaddr 1E:3E:B1:1E:16:05
          inet addr:172.30.22.4  Bcast:172.30.22.255  Mask:255.255.255.0
          UP BROADCAST RUNNING MULTICAST  MTU:1400  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:0
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

$ kubectl exec -it nginx-left-deployment-7476fb75fc-g8brt -n sfc-head -- traceroute -q 1 -I 172.30.22.4
traceroute to 172.30.22.4 (172.30.22.4), 30 hops max, 46 byte packets
 1  172.30.11.3 (172.30.11.3)  2.263 ms
 2  172.30.33.3 (172.30.33.3)  2.116 ms
 3  172.30.44.3 (172.30.44.3)  1.196 ms
 4  172.30.22.4 (172.30.22.4)  1.247 ms
 ```
 ### Flow II
 Before testing the next feature, please make sure the calico node and calico kube controller are up and running. Please refer the [Troubleshooting](https://projectcalico.docs.tigera.io/maintenance/troubleshoot/troubleshooting) for more info.

Let trace the packet from left pod to the Internet. The packet flow through the chain and then to virtual router(tm2) and reach the Internet.
If your setup up is behind the proxy. Please take care of your proxy setup before running these testing.
 ```
$ kubectl exec -it nginx-left-deployment-7476fb75fc-g8brt -n sfc-head -- traceroute -q 1 -I google.com
traceroute to google.com (142.251.40.142), 30 hops max, 46 byte packets
 1  172.30.11.3 (172.30.11.3)  0.655 ms
 2  172.30.33.3 (172.30.33.3)  0.589 ms
 3  172.30.44.3 (172.30.44.3)  0.447 ms
 4  172.30.20.2 (172.30.20.2)  2.192 ms
 5  *
 6  10.11.16.1 (10.11.16.1)  1.487 ms
 7  opnfv-gateway.iol.unh.edu (132.177.125.249)  1.152 ms
 8  rcc-iol-gw.iol.unh.edu (132.177.123.1)  1.355 ms
 9  fatcat.unh.edu (132.177.100.4)  1.802 ms
10  unh-cps-nox300gw1.nox.org (18.2.128.85)  5.060 ms
11  192.5.89.38 (192.5.89.38)  11.580 ms
12  18.2.145.18 (18.2.145.18)  11.306 ms
13  108.170.248.65 (108.170.248.65)  12.086 ms
14  216.239.49.65 (216.239.49.65)  12.083 ms
15  lga25s80-in-f14.1e100.net (142.251.40.142)  11.893 ms
```
### Flow III
Let trace the packet from the server tm1-node to Internet through SFC
```
vagrant@tm1-node:~$ sudo traceroute -q 1 -I google.com
traceroute to google.com (142.251.32.110), 30 hops max, 60 byte packets
 1  172.30.10.3 (172.30.10.3)  2.417 ms
 2  172.30.33.3 (172.30.33.3)  2.344 ms
 3  172.30.44.3 (172.30.44.3)  2.340 ms
 4  172.30.20.2 (172.30.20.2)  2.427 ms
 5  *
 6  _gateway (10.11.16.1)  2.916 ms
 7  opnfv-gateway.iol.unh.edu (132.177.125.249)  2.950 ms
 8  rcc-iol-gw.iol.unh.edu (132.177.123.1)  2.942 ms
 9  fatcat.unh.edu (132.177.100.4)  3.782 ms
10  unh-cps-nox300gw1.nox.org (18.2.128.85)  54.561 ms
11  192.5.89.38 (192.5.89.38)  60.236 ms
12  18.2.145.18 (18.2.145.18)  60.196 ms
13  108.170.248.65 (108.170.248.65)  60.480 ms
14  142.251.60.181 (142.251.60.181)  61.927 ms
15  lga25s77-in-f14.1e100.net (142.251.32.110)  61.907 ms
```
### Flow IV
Let trace packet from pod to tm1-node server through SFC
```
$ kubectl exec -it nginx-right-deployment-965b96d57-65bvx -n sfc-tail -- traceroute -q 1 -I 172.30.10.101
traceroute to 172.30.10.101 (172.30.10.101), 30 hops max, 46 byte packets
 1  172.30.22.3 (172.30.22.3)  0.996 ms
 2  172.30.44.2 (172.30.44.2)  0.689 ms
 3  172.30.33.2 (172.30.33.2)  0.717 ms
 4  172.30.10.101 (172.30.10.101)  1.137 ms
```
## License

Apache-2.0

[1]: https://www.vagrantup.com/
[2]: https://www.vagrantup.com/docs/cli/
