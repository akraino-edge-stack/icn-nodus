# v5.0.0

## Downloads for v5.0.0

### Source Code

filename | sha512 hash
-------- | -----------
[icn-nodus-5.0.0.tar.gz](https://github.com/akraino-edge-stack/icn-nodus/archive/refs/tags/v5.0.0.tar.gz) | 64f315cae5e8186577a12868254017702db27849d2b54f4559978b6a6c31ce5c376740c1697a3f1f56636211e5953b953a9cf0dfcc87cdfe9316590f420107f1
### Container Images

name |
---- |
[integratedcloudnative/ovn4nfv-k8s-plugin:v5.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |
[integratedcloudnative/ovn-images:v2.2.0](https://hub.docker.com/r/integratedcloudnative/ovn-images/tags) |

## Changelog since v4.0.0
### Feature

- Following features added
  - Added feature to support multicast router in distance vector multicast routing protocol
  - Added the CNI packages in the Nodus docker images to secure the deployment issues
  - Updated the Nodus Primary network SFC and SDEWAN testing demo script
  - Added the pool pkg to support dynamic subnet calculation from user config
  - Supported dynamic pool networks using network label with prefix "net" in SFC
  - Added multiple Pod Selector in SFC CRD
  - Added the mutex locking mechanism to access the network pool through multiple SFC threats
  - Deprecated the CRD apiversion "apiextensions.k8s.io/v1beta1" to support "apiextensions.k8s.io/v1"
  - Deprecated the OVN Centos Support from Nodus

### Bug

- Fixed SFC labels to support kubernetes fqdn format
- Fixed the Nodus CNI filename to avoid conflict in secondary network mode
- Removed the deprecated CRD and centos OVN yamls
