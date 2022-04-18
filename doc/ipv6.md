# IPv6 support

- [IPv6 support](#ipv6-support)
  - [Supported modes](#supported-modes)
  - [Deploying Nodus in various modes](#deploying-nodus-in-various-modes)
    - [IPv4 only mode](#ipv4-only-mode)
    - [IPv4/IPv6 dualstack mode](#ipv4ipv6-dualstack-mode)
    - [IPv6/IPv4 dualstack mode](#ipv6ipv4-dualstack-mode)
    - [IPv6 only mode](#ipv6-only-mode)

## Supported modes

Currently Nodus supports 4 modes of operation:

- IPv4 only
- IPv4/IPv6 - dualstack mode where IPv4 is a primary protocol
- IPv6/IPv4 - dualstack mode where IPv6 is a primary protocol
- IPv6 only

Mode of operation can be set by creating Kubernetes cluster operating in suitable mode and setting Nodus deployment environmental variables.

## Deploying Nodus in various modes

> NOTE: In all provided examples you can change the podSubnet and serviceSubnet values so those would fit requirements for your network. If so, please remeber to update OVN_SUBNET, OVN_GATEWAY, OVN_SUBNET_V6 and OVN_GATEWAY_V6 accordingly. You can find more details below.

### IPv4 only mode

For IPv4 only deployment please follow the [Nodus installation quickstart guide](https://github.com/akraino-edge-stack/icn-nodus#quickstart-installation-guide).

### IPv4/IPv6 dualstack mode

1. Enable IPv6 support in your node's OS.

    ```
    sysctl -w net.ipv6.conf.all.disable_ipv6=0
    sysctl -w net.ipv6.conf.default.disable_ipv6=0
    sysctl -w net.ipv6.conf.all.forwarding=1
    ```

2. Create kubeadm ClusterConfiguration config file with the following content:

    ```
    kind: InitConfiguration
    localAPIEndpoint:
      advertiseAddress: <control_plane_ipv4_address>
    nodeRegistration:
      criSocket: /var/run/dockershim.sock
      name: <control_plane_hostname>
      kubeletExtraArgs:
        node-ip: <control_plane_ipv4_address>
    ---
    kind: ClusterConfiguration
    apiVersion: kubeadm.k8s.io/v1beta2
    featureGates:
      IPv6DualStack: true
    networking:
      podSubnet: 10.151.142.0/18,bef0:1234:a890:5678::/123
      serviceSubnet: 10.96.0.0/12,bef0:1234:a890:5679::/123
    controllerManager:
      extraArgs:
        node-cidr-mask-size-ipv4: '19'
        node-cidr-mask-size-ipv6: '124'
    ```
    Type of the first address in `podSubnet` and `serviceSubnet` will determine the protocol that will be used as primary (IPv4 here)

    >NOTE: Please remember to change \<control_plane_ipv4_address\> and \<control_plane_hostname\> to values suitable for your node.

3. Update Nodus deployment file `deploy/ovn4nfv-k8s-plugin.yaml` by uncommenting variables `OVN_SUBNET_V6` and `OVN_GATEWAYIP_V6` in `ovn-controller-network` ConfigMap. Change their values, if required, so they would be the same as in the previously created config file.

4. Initialize cluster using kubeadm init command:

    ```
    kubeadm init --config=<config_file_path>
    ```

5. Follow the steps in the [Nodus installation quickstart guide](https://github.com/akraino-edge-stack/icn-nodus#quickstart-installation-guide) - remove taints, label control-plane-node and deploy OVN daemonset and Nodus deployment.

### IPv6/IPv4 dualstack mode

1. Enable IPv6 support in your node's OS.

    ```
    sysctl -w net.ipv6.conf.all.disable_ipv6=0
    sysctl -w net.ipv6.conf.default.disable_ipv6=0
    sysctl -w net.ipv6.conf.all.forwarding=1
    ```

2. Create kubeadm ClusterConfiguration config file with the following content:

    ```
    kind: InitConfiguration
    localAPIEndpoint:
      advertiseAddress: <control_plane_ipv6_address>
    nodeRegistration:
      criSocket: /var/run/dockershim.sock
      name: <control_plane_hostname>
      kubeletExtraArgs:
        node-ip: <control_plane_ipv6_address>
    ---
    kind: ClusterConfiguration
    apiVersion: kubeadm.k8s.io/v1beta2
    featureGates:
      IPv6DualStack: true
    networking:
      podSubnet: bef0:1234:a890:5678::/123,10.151.142.0/18
      serviceSubnet: bef0:1234:a890:5679::/123,10.96.0.0/12
    controllerManager:
      extraArgs:
        node-cidr-mask-size-ipv4: '19'
        node-cidr-mask-size-ipv6: '124'
    ```
    Type of the first address in `podSubnet` and `serviceSubnet` will determine the protocol that will be used as primary (IPv6 here)

    >NOTE: Please remember to change \<control_plane_ipv6_address\> and \<control_plane_hostname\> to values suitable for your node.

3. Update Nodus deployment file `deploy/ovn4nfv-k8s-plugin.yaml` by uncommenting variables `OVN_SUBNET_V6` and `OVN_GATEWAYIP_V6` in `ovn-controller-network` ConfigMap. Change their values, if required, so they would be the same as in the previously created config file.

4. Initialize cluster using kubeadm init command:

    ```
    kubeadm init --config=<config_file_path>
    ```

5. Follow the steps in the [Nodus installation quickstart guide](https://github.com/akraino-edge-stack/icn-nodus#quickstart-installation-guide) - remove taints, label control-plane-node and deploy OVN daemonset and Nodus deployment.

### IPv6 only mode

1. Enable IPv6 support in your node's OS.

    ```
    sysctl -w net.ipv6.conf.all.disable_ipv6=0
    sysctl -w net.ipv6.conf.default.disable_ipv6=0
    sysctl -w net.ipv6.conf.all.forwarding=1
    ```

2. Create kubeadm ClusterConfiguration config file with the following content:

    ```
    kind: InitConfiguration
    localAPIEndpoint:
      advertiseAddress: <control_plane_ipv6_address>
    nodeRegistration:
      criSocket: /var/run/dockershim.sock
      name: <control_plane_hostname>
      kubeletExtraArgs:
        node-ip: <control_plane_ipv6_address>
    ---
    kind: ClusterConfiguration
    apiVersion: kubeadm.k8s.io/v1beta2
    featureGates:
      IPv6DualStack: true
    networking:
      podSubnet: bef0:1234:a890:5678::/123
      serviceSubnet: bef0:1234:a890:5679::/123
    controllerManager:
      extraArgs:
        node-cidr-mask-size-ipv6: '124'
    ```
    AS only IPv6 subnet is specified in `podSubnet` and `serviceSubnet` Kubernetes cluster will be created in IPv6 only mode.

    >NOTE: Please remember to change \<control_plane_ipv6_address\> and \<control_plane_hostname\> to values suitable for your node.

3. Update Nodus deployment file `deploy/ovn4nfv-k8s-plugin.yaml` by removing or commenting out variables `OVN_SUBNET` and `OVN_GATEWAY` and uncommenting variables `OVN_SUBNET_V6` and `OVN_GATEWAYIP_V6` in `ovn-controller-network` ConfigMap. Change their values, if required, so they would be the same as in the previously created config file.

4. Initialize cluster using kubeadm init command:

    ```
    kubeadm init --config=<config_file_path>
    ```

5. Follow the steps in the [Nodus installation quickstart guide](https://github.com/akraino-edge-stack/icn-nodus#quickstart-installation-guide) - remove taints, label control-plane-node and deploy OVN daemonset and Nodus deployment.