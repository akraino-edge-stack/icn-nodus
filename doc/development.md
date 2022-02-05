# Developer Information

## How to build the Nodus?

Nodus is maintained in icn gerrit repositry - [https://gerrit.akraino.org/r/admin/repos/icn/nodus](https://gerrit.akraino.org/r/admin/repos/icn/nodus)

The github page [https://github.com/akraino-edge-stack/icn-nodus/](https://github.com/akraino-edge-stack/icn-nodus/)
is github mirror repository for the user.

Contributor login with Linux Foundation ID into [https://gerrit.akraino.org/r/admin/repos/icn/nodus](https://gerrit.akraino.org/r/admin/repos/icn/nodus)
and add their ssh key or use the LF username and password

### How to build Nodus entity?

Before packaging the Nodus images, please make sure your feature request or
modification are compilable as follows:

```
# git clone https://github.com/akraino-edge-stack/icn-nodus.git
# make all
```

you can use gerrit anonymous https clone or clone with commit-msg hook. Above example show the clone with github

There are 3 components within the nodus - `nfn-operator`, `ovn4nfvk8s-cni` & `nfn-agent`.

All these components can be modified and build separately using the commands -  `make nfn-operator`, `make ovn4nfvk8s-cni` &
`make nfn-agent`

All the components can be cleaned with `make clean`

### How to run the CI tests?

Nodus has go unit test for all golang package. For CNI testing is based on ginkgo framework.
The following command run the unit tests and it is intergrated with opnfv gerrit review to give +1

```
# make test
```

### How to build docker images?

The nodus project has 2 major docker images for ovn/ovs operations and network
controller. The nodus currently support and tested in ubuntu and centos distro.
In this documentation, developer get the information regarding how to build and
package the docker images

- ovn dockerfile is maintained for centos and ubuntu package in the `utilities/docker` folder
- nodus k8s plugin dockerfile is maintained for centos and ubuntu in the `build` folder

#### ubuntu distro
Build the ovn-images
```
pushd utilities/docker && \
docker build --no-cache --rm -t integratedcloudnative/ovn-images:master . -f debian/Dockerfile && \
popd
```
Build the nodus images
```
docker build --no-cache --rm -t integratedcloudnative/ovn4nfv-k8s-plugin:master . -f build/Dockerfile && \
```

docker images are tagged with intergratedcloudnative dockerhub.
For development purpose use your private dockerhub, if planning to test k8s deployment is more than one node.

#### centos distro
Build the ovn-images
```
pushd utilities/docker && \
docker build --no-cache --rm -t integratedcloudnative/ovn-images:master . -f centos/Dockerfile && \
popd
```
Build the nodus images
```
docker build --no-cache --rm -t integratedcloudnative/ovn4nfv-k8s-plugin:master . -f build/Dockerfile.centos && \
```

### How to deploy the compiled nodus docker images in k8s cluster?

K8s yaml files in the `deploy` folder use that stable release of the ovn-images and ovn4nfv-k8s-plugin.
Please change the images in the yaml, if you deploying your compiled nodus images

Please follow the steps - [quickstart-installation-guide](https://github.com/akraino-edge-stack/icn-nodus#quickstart-installation-guide)

For the kubespray change the ansible variable - `ovn4nfv_ovn_image_version`, `ovn4nfv_k8s_plugin_image_version`,
`ovn4nfv_ovn_image_repo`, `ovn4nfv_k8s_plugin_image_repo` in the kubespray `roles/download/defaults/main.yml`

### How to test the compiled nodus docker images in k8s cluster?

Please follow the steps -[doc/how-to-use.md#network-testing](https://github.com/akraino-edge-stack/icn-nodus/blob/master/doc/how-to-use.md#network-testing)
