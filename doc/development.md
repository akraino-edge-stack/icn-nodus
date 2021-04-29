# Developer Information

- [Developer Information](#developer-information)
  - [How to build the ovn4nfv?](#how-to-build-the-ovn4nfv)
    - [How to build ovn4nfv entity?](#how-to-build-ovn4nfv-entity)
    - [How to run the CI tests?](#how-to-run-the-ci-tests)
    - [How to build docker images?](#how-to-build-docker-images)
      - [Ubuntu distro](#ubuntu-distro)
      - [CentOS distro](#centos-distro)
    - [How to deploy the compiled ovn4nfv docker images in k8s cluster?](#how-to-deploy-the-compiled-ovn4nfv-docker-images-in-k8s-cluster)
    - [How to test the compiled ovn4nfv docker images in k8s cluster?](#how-to-test-the-compiled-ovn4nfv-docker-images-in-k8s-cluster)

## How to build the ovn4nfv?

ovn4nfv-k8s-plugin is maintained in [opnfv gerrit repositry](https://gerrit.opnfv.org/gerrit/admin/repos/ovn4nfv-k8s-plugin).

[The github page](https://github.com/opnfv/ovn4nfv-k8s-plugin/)
is github mirror repository for the user.

Contributor login with Linux Foundation ID into [Gerrit](https://gerrit.opnfv.org/gerrit/admin/repos/ovn4nfv-k8s-plugin)
and add their ssh key or use the LF username and password

### How to build ovn4nfv entity?

Before packaging the ovn4nfv images, please make sure your feature request or
modification are compilable as follows:

```bash
git clone https://github.com/opnfv/ovn4nfv-k8s-plugin.git
make all
```

you can use gerrit anonymous http clone or clone with commit-msg hook. Above example show the clone with github

There are 3 components within the ovn4nfv-k8s-plugin - `nfn-operator`, `ovn4nfvk8s-cni` & `nfn-agent`.

All these components can be modified and build separately using the commands - `make nfn-operator`, `make ovn4nfvk8s-cni` &
`make nfn-agent`

All the components can be cleaned with `make clean`

### How to run the CI tests?

ovn4nfv-k8s-plugin has go unit test for all golang package. For CNI testing is based on ginkgo framework.
The following command run the unit tests and it is intergrated with opnfv gerrit review to give +1

```bash
make test
```

### How to build docker images?

The ovn4nfv project has 2 major docker images for ovn/ovs operations and network
controller. The ovn4nfv currently support and tested in ubuntu and centos distro.
In this documentation, developer get the information regarding how to build and
package the docker images

- ovn dockerfile is maintained for centos and ubuntu package in the `utilities/docker` folder
- ovn4nfv k8s plugin dockerfile is maintained for centos and ubuntu in the `build` folder

#### Ubuntu distro

Build the ovn-images

```bash
pushd utilities/docker && \
docker build --no-cache --rm -t integratedcloudnative/ovn-images:master . -f debian/Dockerfile && \
popd
```

Build the ovn4nfv-k8s-plugin images

```bash
docker build --no-cache --rm -t integratedcloudnative/ovn4nfv-k8s-plugin:master . -f build/Dockerfile && \
```

docker images are tagged with intergratedcloudnative dockerhub.
For development purpose use your private dockerhub, if planning to test k8s deployment is more than one node.

#### CentOS distro

Build the ovn-images

```bash
pushd utilities/docker && \
docker build --no-cache --rm -t integratedcloudnative/ovn-images:master . -f centos/Dockerfile && \
popd
```

Build the ovn4nfv-k8s-plugin images

```bash
docker build --no-cache --rm -t integratedcloudnative/ovn4nfv-k8s-plugin:master . -f build/Dockerfile.centos && \
```

### How to deploy the compiled ovn4nfv docker images in k8s cluster?

K8s yaml files in the `deploy` folder use that stable release of the ovn-images and ovn4nfv-k8s-plugin.
Please change the images in the yaml, if you deploying your compiled ovn4nfv images

Please follow the steps - [quickstart-installation-guide](https://github.com/opnfv/ovn4nfv-k8s-plugin#quickstart-installation-guide)

For the kubespray change the ansible variable - `ovn4nfv_ovn_image_version`, `ovn4nfv_k8s_plugin_image_version`,
`ovn4nfv_ovn_image_repo`, `ovn4nfv_k8s_plugin_image_repo` in the kubespray `roles/download/defaults/main.yml`

### How to test the compiled ovn4nfv docker images in k8s cluster?

Please follow the steps -[doc/how-to-use.md#network-testing](https://github.com/opnfv/ovn4nfv-k8s-plugin/blob/master/doc/how-to-use.md#network-testing)
