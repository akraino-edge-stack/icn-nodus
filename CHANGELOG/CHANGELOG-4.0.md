# v4.0.0

## Downloads for v4.0.0

### Source Code

filename | sha512 hash
-------- | -----------
[icn-nodus-4.0.0.tar.gz](https://github.com/akraino-edge-stack/icn-nodus/archive/refs/tags/v4.0.0.tar.gz) | 743ab1ecc97da9d845a73bb3e9b6d644cddf78a6ac82e89c188f0284d10d06f25f823f5e37edae089a623691bf46d425bb4895172bffa169e273f1c2741b2dc0
### Container Images

name |
---- |
[integratedcloudnative/ovn4nfv-k8s-plugin:v4.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |
[integratedcloudnative/ovn-images:v2.2.0](https://hub.docker.com/r/integratedcloudnative/ovn-images/tags) |

## Changelog since v3.0.0
### Feature

- Following features added
  - Added hostnetwork, svc network, and pod network routes in the podSelector pods
  - Added subnet/supernet calculation package to support dynamic network creation
  - Added dynamic virtual network creation in Nodus using network pool mechanism
  - Added dynamic SFC deployment features to support EMCO deployment.
  - Added validation features in Nodus to check the container state before deploying SFC in pod network namespace
  
### Bug

- Fixed the nodus internal pkg and github pkg url
- Fixed host network, svc network and pod network routing in pod group in SFC with primary network gw ip
- Fixed the bug to support multiple podSelector in SFC CR