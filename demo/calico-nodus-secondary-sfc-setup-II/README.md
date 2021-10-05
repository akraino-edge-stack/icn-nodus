# Service Function Chaining(SFC) - setup

## Summary

This project offers a means for deploying a Kubernetes cluster
that satisfies the requirements of ovn4nfv sfc-setup

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
In the master node run the `kubeadm init` as below. The calico uses pod network cidr `10.233.64.0/18`
```
    $ kubeadm init --kubernetes-version=1.19.0 --pod-network-cidr=10.233.64.0/18 --apiserver-advertise-address=<master_eth0_ip_address>
```
Ensure the master node taint for no schedule is removed and labelled with `ovn4nfv-k8s-plugin=ovn-control-plane`
```
nodename=$(kubectl get node -o jsonpath='{.items[0].metadata.name}')
kubectl taint node $nodename node-role.kubernetes.io/master:NoSchedule-
kubectl label --overwrite node $nodename ovn4nfv-k8s-plugin=ovn-control-plane
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

In this example, we are using pod CIDR as `10.233.64.0/18`. The Calico will automatically detect the CIDR based on the running configuration.
Since calico network going to the primary network in our case, ovn4nfv subnet should be a different network. Make sure you change the `ovn_subnet` and `ovn_gatewayip` in `deploy/ovn4nfv-k8s-plugin-sfc-setup-II.yaml`. Setup `Network` and `SubnetLen`as per user configuration.

In this example, we customize the ovn network as follows.
```
data:
  ovn_subnet: "10.154.142.0/18"
  ovn_gatewayip: "10.154.142.1/18"
  virtual-net-conf.json: |
    {
      "Network": "172.30.16.0/22",
      "SubnetLen": 24
    }```
Deploy the Nodus Pod network to the cluster.
```
    $ kubectl apply -f deploy/ovn-daemonset.yaml
    $ kubectl apply -f deploy/ovn4nfv-k8s-plugin-sfc-setup-II.yaml
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
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sfc-private-network.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/slb-multiple-network.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/ngfw.yaml
   $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sdewan-multiple-network.yaml
```
- The above commends created the multiple networks - provider-network-1 and provider-network-2. The corresponding vlan tagging is created in the nodes
- Dummy VFs application are deployed in this case are Smart Load balancer,Next Generation Firewall and Software Defined
Edge WAN. This could be replaced by the actual VFs application.

Next steps to deploy Pods and deploy the SFCs

```
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/nginx-left-deployment.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/nginx-right-deployment.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sfc-with-only-nf-labels.yaml
```
Let trace the packet flow in the sfc for the internal and external traffic throug sfc

```
   $ kubectl get pods -A -o wide
NAMESPACE     NAME                                      READY   STATUS    RESTARTS   AGE     IP               NODE       NOMINATED NODE   READINESS GATES
default       ngfw-6bccb7789-9f92p                      1/1     Running   0          3m39s   10.233.89.240    minion21   <none>           <none>
default       sdewan-5866d596c9-phpnt                   1/1     Running   0          2m51s   10.233.89.241    minion21   <none>           <none>
default       slb-759ff8df84-2spc2                      1/1     Running   0          4m18s   10.233.89.239    minion21   <none>           <none>
kube-system   calico-kube-controllers-c9784d67d-xbr99   1/1     Running   0          40d     10.233.89.133    minion21   <none>           <none>
kube-system   calico-node-vng88                         1/1     Running   0          38d     192.168.121.19   minion21   <none>           <none>
kube-system   calico-node-vq69d                         1/1     Running   0          38d     192.168.121.23   master20   <none>           <none>
kube-system   coredns-f9fd979d6-6lq9s                   1/1     Running   0          40d     10.233.89.132    minion21   <none>           <none>
kube-system   coredns-f9fd979d6-xgw9q                   1/1     Running   0          40d     10.233.95.66     master20   <none>           <none>
kube-system   etcd-master20                             1/1     Running   3          40d     192.168.121.23   master20   <none>           <none>
kube-system   kube-apiserver-master20                   1/1     Running   3          40d     192.168.121.23   master20   <none>           <none>
kube-system   kube-controller-manager-master20          1/1     Running   27         40d     192.168.121.23   master20   <none>           <none>
kube-system   kube-multus-ds-amd64-2m2lr                1/1     Running   1          40d     192.168.121.23   master20   <none>           <none>
kube-system   kube-multus-ds-amd64-88d7v                1/1     Running   1          40d     192.168.121.19   minion21   <none>           <none>
kube-system   kube-proxy-k7gcv                          1/1     Running   1          40d     192.168.121.23   master20   <none>           <none>
kube-system   kube-proxy-zvwn9                          1/1     Running   1          40d     192.168.121.19   minion21   <none>           <none>
kube-system   kube-scheduler-master20                   1/1     Running   27         40d     192.168.121.23   master20   <none>           <none>
kube-system   nfn-agent-hhlnk                           1/1     Running   0          6m20s   192.168.121.19   minion21   <none>           <none>
kube-system   nfn-agent-qwpzr                           1/1     Running   0          6m20s   192.168.121.23   master20   <none>           <none>
kube-system   nfn-operator-d48b44586-mkkl7              1/1     Running   0          6m20s   192.168.121.23   master20   <none>           <none>
kube-system   ovn-control-plane-cc67d7668-6z2lp         1/1     Running   1          40d     192.168.121.23   master20   <none>           <none>
kube-system   ovn-controller-chnmn                      1/1     Running   1          40d     192.168.121.23   master20   <none>           <none>
kube-system   ovn-controller-dqm4b                      1/1     Running   1          40d     192.168.121.19   minion21   <none>           <none>
kube-system   ovn4nfv-cni-8btdz                         1/1     Running   0          6m20s   192.168.121.23   master20   <none>           <none>
kube-system   ovn4nfv-cni-cf49n                         1/1     Running   0          6m20s   192.168.121.19   minion21   <none>           <none>
sfc-head      nginx-left-deployment-76c9bb4ff-98ww7     1/1     Running   0          98s     10.233.95.101    master20   <none>           <none>
sfc-head      nginx-left-deployment-76c9bb4ff-gd745     1/1     Running   0          98s     10.233.89.242    minion21   <none>           <none>
sfc-head      nginx-left-deployment-76c9bb4ff-mq97r     1/1     Running   0          98s     10.233.89.243    minion21   <none>           <none>
sfc-tail      nginx-right-deployment-f6cfb7679-btpwx    1/1     Running   0          78s     10.233.95.102    master20   <none>           <none>
sfc-tail      nginx-right-deployment-f6cfb7679-rrdbc    1/1     Running   0          78s     10.233.89.245    minion21   <none>           <none>
sfc-tail      nginx-right-deployment-f6cfb7679-shggl    1/1     Running   0          78s     10.233.89.244    minion21   <none>           <none>
```
Let ping from the left pod to right pod and left pod to internet
```
   $ kubectl exec -it nginx-right-deployment-f6cfb7679-btpwx -n sfc-tail -- ifconfig
eth0      Link encap:Ethernet  HWaddr 46:D3:84:3A:5F:14
          inet addr:10.233.95.102  Bcast:10.233.95.102  Mask:255.255.255.255
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

sn0       Link encap:Ethernet  HWaddr 0E:95:83:1E:13:05
          inet addr:172.30.19.4  Bcast:172.30.19.255  Mask:255.255.255.0
          UP BROADCAST RUNNING MULTICAST  MTU:1400  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:0
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

$ kubectl exec -it nginx-left-deployment-76c9bb4ff-98ww7 -n sfc-head -- traceroute -n -q 1 -I 172.30.19.4
traceroute to 172.30.19.4 (172.30.19.4), 30 hops max, 46 byte packets
 1  172.30.16.3  3.051 ms
 2  172.30.17.4  2.589 ms
 3  172.30.18.4  1.860 ms
 4  172.30.19.4  2.855 ms
```
## License

Apache-2.0

[1]: https://www.vagrantup.com/
[2]: https://www.vagrantup.com/docs/cli/
