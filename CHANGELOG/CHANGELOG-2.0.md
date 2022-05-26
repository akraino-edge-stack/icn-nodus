# v2.0.0

## Downloads for v2.0.0

### Source Code

filename | sha512 hash
-------- | -----------
[icn-nodus-2.0.0.tar.gz](https://github.com/akraino-edge-stack/icn-nodus/archive/refs/tags/v2.0.0.tar.gz) | 94e0e01d2e40b0efc514973a869402a3fe3792693e3d3132d09915fde6443f09aca32f9e30ed5fd984870130b5c9baa72ac1ebfc23e26da27022414ff5fc1f72
### Container Images

name |
---- |
[integratedcloudnative/ovn4nfv-k8s-plugin:v2.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |
[integratedcloudnative/ovn4nfv-k8s-plugin:centos-v2.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |
[integratedcloudnative/ovn-images:v2.0.0](https://hub.docker.com/r/integratedcloudnative/ovn-images/tags) |
[integratedcloudnative/ovn-images:centos-v2.0.0](https://hub.docker.com/r/integratedcloudnative/ovn-images/tags) |


## Changelog since v1.0.0
### Feature

- Following features added
  - Adding the Primary network feature in Nodus and removing the dependence on cni proxy plugin
  - Nodus creates ovn overlay multi-networking using pod annotations itself
  - Adding ovn containerization and ovn daemonset deployment in nodus and removing the dependencies on standalone ovn installation on the host
  - Divided the CNI plugin into two parts CNI server and CNI shim. CNI sever resides in nfn-agent
  - Added the Nodus gateway interfaces, OVN node switch port, and SNAT rules for Nodus gateway interfaces
  - Added SFC controller based on route based chaining
  - Added iface pkg in nodus
  - Added gwipaddress features in network annotation
  - Added centos 8 support for Nodus and OVN docker images
  - update the documentation on development.md and configuration.md

### Bug

- Removed the outdated unit test
- Fixing the insync message issues in nfn-agent
- Fixing the broken link in readme.md