# v6.0.0

## Downloads for v6.0.0

### Source Code

filename | sha512 hash
-------- | -----------
[icn-nodus-6.0.0.tar.gz](https://github.com/akraino-edge-stack/icn-nodus/archive/refs/tags/v6.0.0.tar.gz) | 564fa4a9ded91676b48ecd0a32fd6fcc7a4e9dd784001d29710314a331edc9d175fae8ce9002c1c92469046c8107ae48950b7a6e26aafde8c6138faa3f92daf5
### Container Images

name |
---- |
[integratedcloudnative/ovn4nfv-k8s-plugin:v6.0.0](https://hub.docker.com/r/integratedcloudnative/ovn4nfv-k8s-plugin/tags) |
[integratedcloudnative/ovn-images:v2.3.0](https://hub.docker.com/r/integratedcloudnative/ovn-images/tags) |

## Changelog since v5.0.0
### Feature

- Following features added
  - Updated the Docker images to have a multi-stage build with a golang base image
  - Update client-go package to support k8s 1.23
  - updated K8S API to k8s 1.23
  - Added optimization feature for user to select the host interface for the OVS binding in Nodus
  - Added IPv6 feature to support the Kubernetes dual-stack mode with IPv4 as the primary IP address and IPv6 as the secondary IP address or IPv6 as the primary IP address and IPv4 as the secondary IP address and IPv6 only mode. Please see this [guide](https://github.com/akraino-edge-stack/icn-nodus/blob/master/doc/ipv6.md)
  - Developed the OVN ACL  and OVN port group golang package in Nodus
  - Developed Kubernetes network policy controller based on the OVN ACLs to implement the control traffic flow based on the OVN port group concept. Kubernetes network policies control traffic flow at the IP address or port level(OSI Layer 3 or 4). Please see this [guide](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
  - Developed Fuzz tool using radamsa using to validate all YAML input fields in Nodus CR. Please see the [test script](https://github.com/akraino-edge-stack/icn-nodus/blob/master/testing/fuzzing/test.sh)
  - Integrated Cert Manager in Nodus to generate and store certificate, signature key, and CA certificates for the mTLS connectivity as k8s secrets
  - Developed auth pkg in Nodus to use Cert Manager secrets to establish minimum TLSv1.2 and cipher TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384 across all entity in Nodus to secure the OVN and Nodus control data.
  
### Bug

- Upgraded protobuf package version to 1.3.2 to fix the vulnerability [CVE-2021-3121](https://github.com/advisories/GHSA-c3h9-896r-86jm) in 1.3.1
- Fixed nodus build issue by deprecating the go-openapi/spec package and used kube-openapi/pkg/validation/spec in Nodus
- Fixed interface creation issue in PodSelectors in SFC by adding both IPv4 and IPv6 data fields
- Removed the BDBA high and medium vulnerability in both OVN and Nodus docker images
- Removed the Synk scan high and medium vulnerability in Nodus
- Remove the privileges access in the Nodus pods