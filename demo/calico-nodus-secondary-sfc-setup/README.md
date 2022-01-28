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
Since calico network going to the primary network in our case, ovn4nfv subnet should be a different network. Make sure you change the `OVN_SUBNET` and `OVN_GATEWAYIP` in `deploy/ovn4nfv-k8s-plugin.yaml`
In this example, we customize the ovn network as follows.                      
```                                                                            
data:                                                                          
  OVN_SUBNET: "10.154.142.0/18"                                                
  OVN_GATEWAYIP: "10.154.142.1/18"                                             
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
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/namespace-right
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/namespace-left
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/nginx-left-deployment.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/nginx-right-deployment.yaml
    $ kubectl apply -f demo/calico-nodus-secondary-sfc-setup/deploy/sfc-with-virtual-and-provider-network.yaml
```
Let trace the packet flow in the sfc for the internal and external traffic throug sfc

```
   $ kubectl get pods -A -o wide                                                                                                                                           
NAMESPACE     NAME                                                       READY   STATUS    RESTARTS   AGE     IP               NODE       NOMINATED NODE   READINESS GATES
default       ngfw-64746d5897-mnwvv                                      1/1     Running   0          10m     10.233.105.110   minion01   <none>           <none>
default       sdewan-7db46bdd45-nhh9r                                    1/1     Running   0          9m54s   10.233.105.111   minion01   <none>           <none>
default       slb-598448fb4b-qp6rr                                       1/1     Running   0          10m     10.233.105.109   minion01   <none>           <none>
kube-system   calico-kube-controllers-c9784d67d-ttzd5                    1/1     Running   0          3d22h   10.233.105.80    minion01   <none>           <none>
kube-system   calico-node-58pj2                                          1/1     Running   0          3d16h   192.168.121.26   master     <none>           <none>
kube-system   calico-node-mvntb                                          1/1     Running   0          3d16h   192.168.121.30   minion01   <none>           <none>
kube-system   coredns-f9fd979d6-g88zw                                    1/1     Running   0          3d16h   10.233.97.189    master     <none>           <none>
kube-system   coredns-f9fd979d6-qtc9b                                    1/1     Running   0          3d16h   10.233.105.102   minion01   <none>           <none>
kube-system   etcd-master                                                1/1     Running   1          26d     192.168.121.26   master     <none>           <none>
kube-system   kube-apiserver-master                                      1/1     Running   1          26d     192.168.121.26   master     <none>           <none>
kube-system   kube-controller-manager-master                             1/1     Running   24         26d     192.168.121.26   master     <none>           <none>
kube-system   kube-multus-ds-amd64-9r8b6                                 1/1     Running   1          26d     192.168.121.26   master     <none>           <none>
kube-system   kube-multus-ds-amd64-bdfz5                                 1/1     Running   1          26d     192.168.121.30   minion01   <none>           <none>
kube-system   kube-proxy-lwkm8                                           1/1     Running   1          26d     192.168.121.26   master     <none>           <none>
kube-system   kube-proxy-mj98m                                           1/1     Running   2          26d     192.168.121.30   minion01   <none>           <none>
kube-system   kube-scheduler-master                                      1/1     Running   23         26d     192.168.121.26   master     <none>           <none>
kube-system   nfn-agent-mf6zv                                            1/1     Running   0          12m     192.168.121.30   minion01   <none>           <none>
kube-system   nfn-agent-qfc8d                                            1/1     Running   0          12m     192.168.121.26   master     <none>           <none>
kube-system   nfn-operator-689d79dc69-8fghz                              1/1     Running   0          12m     192.168.121.26   master     <none>           <none>
kube-system   ovn-control-plane-cc67d7668-jqbcv                          1/1     Running   1          26d     192.168.121.26   master     <none>           <none>
kube-system   ovn-controller-cbh8t                                       1/1     Running   1          26d     192.168.121.26   master     <none>           <none>
kube-system   ovn-controller-l8spp                                       1/1     Running   1          26d     192.168.121.30   minion01   <none>           <none>
kube-system   ovn4nfv-cni-hl9nd                                          1/1     Running   0          12m     192.168.121.26   master     <none>           <none>
kube-system   ovn4nfv-cni-qqptj                                          1/1     Running   0          12m     192.168.121.30   minion01   <none>           <none>
sfc-head      nginx-left-deployment-76c9bb4ff-4zz5f                      1/1     Running   0          2m59s   10.233.105.112   minion01   <none>           <none>
sfc-head      nginx-left-deployment-76c9bb4ff-cbg77                      1/1     Running   0          2m59s   10.233.97.193    master     <none>           <none>
sfc-head      nginx-left-deployment-76c9bb4ff-qvxm8                      1/1     Running   0          2m59s   10.233.105.113   minion01   <none>           <none>
sfc-tail      nginx-right-deployment-f6cfb7679-8bcbd                     1/1     Running   0          95s     10.233.97.194    master     <none>           <none>
sfc-tail      nginx-right-deployment-f6cfb7679-ccqqc                     1/1     Running   0          95s     10.233.105.114   minion01   <none>           <none>
sfc-tail      nginx-right-deployment-f6cfb7679-kfsvc                     1/1     Running   0          95s     10.233.97.195    master     <none>           <none>
```
Let ping from the left pod to right pod and left pod to internet
```
$ kubectl exec -it nginx-right-deployment-f6cfb7679-8bcbd -n sfc-tail -- ifconfig
eth0      Link encap:Ethernet  HWaddr DE:59:0B:4C:34:6F                        
          inet addr:10.233.97.194  Bcast:10.233.97.194  Mask:255.255.255.255   
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
                                                                               
sn0       Link encap:Ethernet  HWaddr 02:FD:CC:1E:16:05                        
          inet addr:172.30.22.4  Bcast:172.30.22.255  Mask:255.255.255.0       
          UP BROADCAST RUNNING MULTICAST  MTU:1400  Metric:1                   
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0                   
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0                 
          collisions:0 txqueuelen:0                                            
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)                               
                                                                               
$ kubectl exec -it nginx-left-deployment-76c9bb4ff-4zz5f -n sfc-head -- traceroute -q 1 -I 172.30.22.4
traceroute to 172.30.22.4 (172.30.22.4), 30 hops max, 46 byte packets          
 1  172.30.11.3 (172.30.11.3)  2.061 ms                                        
 2  172.30.33.3 (172.30.33.3)  0.823 ms                                        
 3  172.30.44.3 (172.30.44.3)  1.781 ms                                        
 4  172.30.22.4 (172.30.22.4)  5.090 ms                                        
$ kubectl exec -it nginx-left-deployment-76c9bb4ff-4zz5f -n sfc-head -- traceroute -q 1 -I google.com
traceroute to google.com (142.251.33.78), 30 hops max, 46 byte packets         
 1  172.30.11.3 (172.30.11.3)  1.097 ms                                        
 2  172.30.33.3 (172.30.33.3)  0.627 ms                                        
 3  172.30.44.3 (172.30.44.3)  0.571 ms                                        
 4  172.30.20.2 (172.30.20.2)  2.847 ms                                        
 5  *                                                                          
 6  10.10.110.1 (10.10.110.1)  1.545 ms                                        
 7  192.55.66.2 (192.55.66.2)  1.697 ms                                        
 8  10.54.2.45 (10.54.2.45)  35.393 ms                                         
 9  10.128.161.137 (10.128.161.137)  1.319 ms                                  
10  *                                                                          
11  *                                                                          
12  *                                                                          
13  *                                                                          
14  *                                                                          
15  *                                                                          
16  *                                                                          
17  *                                                                          
18  sea09s28-in-f14.1e100.net (142.251.33.78)  8.434 ms               
```
## License

Apache-2.0

[1]: https://www.vagrantup.com/
[2]: https://www.vagrantup.com/docs/cli/
