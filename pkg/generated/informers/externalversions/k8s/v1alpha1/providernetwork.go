/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"
	versioned "github.com/akraino-edge-stack/icn-nodus/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/akraino-edge-stack/icn-nodus/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/generated/listers/k8s/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// ProviderNetworkInformer provides access to a shared informer and lister for
// ProviderNetworks.
type ProviderNetworkInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ProviderNetworkLister
}

type providerNetworkInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewProviderNetworkInformer constructs a new informer for ProviderNetwork type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewProviderNetworkInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredProviderNetworkInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredProviderNetworkInformer constructs a new informer for ProviderNetwork type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredProviderNetworkInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.K8sV1alpha1().ProviderNetworks(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.K8sV1alpha1().ProviderNetworks(namespace).Watch(context.TODO(), options)
			},
		},
		&k8sv1alpha1.ProviderNetwork{},
		resyncPeriod,
		indexers,
	)
}

func (f *providerNetworkInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredProviderNetworkInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *providerNetworkInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&k8sv1alpha1.ProviderNetwork{}, f.defaultInformer)
}

func (f *providerNetworkInformer) Lister() v1alpha1.ProviderNetworkLister {
	return v1alpha1.NewProviderNetworkLister(f.Informer().GetIndexer())
}
