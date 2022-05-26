# v3.0.0

## Downloads for v3.0.0

### Source Code

filename | sha512 hash
-------- | -----------
[icn-nodus-3.0.0.tar.gz](https://github.com/akraino-edge-stack/icn-nodus/archive/refs/tags/v3.0.0.tar.gz) | 114d9fca18afa03bebdff6c06a30537043d0daef113511d2c4f3f1ea0d97024ec0a6c82987d79b9641eada1d109f26af3c545506f5d703bb7c8cc5e5e22bb4b4
### Container Images

name |
---- |
[integratedcloudnative/ovn4nfv-k8s-plugin:v3.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |
[integratedcloudnative/ovn-images:v2.2.0](https://hub.docker.com/r/integratedcloudnative/ovn-images/tags) |



## Changelog since v2.0.0
### Feature

- Following features added
  - Upgraded vagrant version and optimized the vagrant installations
  - Added the automated testing
  - Added automated testing for the SFC demo
  - Added K8s service routing for the SFC pods
  - Added the interface hotplug feature in Nodus
  - Added the delete option for the SFC to reverse the SFC modification to reuse the SFC/CNFs pods
  - Added virtual mode in the SFC CRD and used the Kubernetes network labels instead of app name
  - Added validation and counter checking conditions to track the SFC creation for pod before and after SFC implemented
  - Added the pod groups in SFC CRD and interface hot-plugging for the pod groups
  - Added Nodus logo and documentation for the Calico SFC demos
  - Added features to support Kubevirt deployment with Multus to configure only the interface requested in NetworkAttachmentDefinition
  - Added "nfn-network" in the nodus CNI to be called through Multus to support VM deployment with Kubevirt

### Bug

- Fixed the "eth0" interface name conflict between Nodus and CNI proxy plugins
- Fixed the nfn-agent to wait for the ovs service in the host
- Handled the pod deletion appropriately to release deleted ports
- Calico by default doesn't allow the IP forwarding, fixed it in calico YAML
- Fixed the route deletion to check the route existence first
- Fixed the SFC VF routing miscalculation
- Fixed Nodus to support K8s generic pod label format
