# v1.0.0

## Downloads for v1.0.0

### Source Code

filename | sha512 hash
-------- | -----------
[icn-nodus-1.0.0.tar.gz](https://github.com/akraino-edge-stack/icn-nodus/archive/refs/tags/v1.0.0.tar.gz) | aa8e259c4d167724e8814d7529d4b7a43c37196081e2a68008d9cc59ca18cea47e6ee621f557874ba8aed15fcab3511f5608b6c94b89faa0ec877dcfd63b6d92
### Container Images

name |
---- |
[integratedcloudnative/ovn4nfv-k8s-plugin:v1.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |


## Changelog since v0.1.0
### Feature

- Following features added
  - Added initial working Service chaining API and generated code for route based chaining CRD
  - Added direct provider network CRD controller, update the nfn.proto, apis
  - Include direct provider network API in the nfn-agent
  - Modified the nfnNotify server to include the direct provider network
  - Updated the ovn4nfv k8s plugin docker build golang version to v1.14

### Bug

- Fixed the Vlan interface up bug