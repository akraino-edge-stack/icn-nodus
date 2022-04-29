package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"google.golang.org/grpc/credentials"
	kapi "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cm "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
)

const (
	maxTries = 60

	CAFile   = "ca.crt"
	CertFile = "tls.crt"
	KeyFile  = "tls.key"

	CniCertHostPath = "/opt/nodus/certs"
	CniCertPath = "/host/opt/nodus/certs"

	DefaultIssuer  = "nodus-issuer"
	DefaultCert    = "nodus-cert"
	DefaultCniCert = "nodus-cni-cert"

	NamespaceEnv       = "POD_NAMESPACE"
	NfnOperatorHostEnv = "NFN_OPERATOR_SERVICE_HOST"
)

func LoadCertsFromSecret(secret *kapi.Secret) (*tls.Certificate, *x509.CertPool, error) {
	ca := secret.Data[CAFile]
	cert := secret.Data[CertFile]
	key := secret.Data[KeyFile]

	peerCert, err := tls.X509KeyPair(cert, key)
    if err != nil {
        return nil, nil, err
	}

    caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(ca)

	return &peerCert, caCertPool, nil
}

func createTls(secret *kapi.Secret, isServer bool) (*credentials.TransportCredentials, error) {
	peerCert, caCertPool, err := LoadCertsFromSecret(secret)
	if err != nil {
		return nil, err
	}

	var credTLS credentials.TransportCredentials
	if isServer {
		credTLS = credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{*peerCert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})
	} else {
		credTLS = credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{*peerCert},
			RootCAs:      caCertPool,
		})
	}

	return &credTLS, nil
}

// CreateClientTLSFromSecret creates TLS credentials for the GRPC server
func CreateServerTLSFromSecret(secret *kapi.Secret) (*credentials.TransportCredentials, error) {
	return createTls(secret, true)
}

// CreateClientTLSFromSecret creates TLS credentials for the GRPC client
func CreateClientTLSFromSecret(secret *kapi.Secret) (*credentials.TransportCredentials, error) {
	return createTls(secret, false)
}

// CreateClientTLSConfig creates TLS config for the client
func CreateClientTLSConfig(secret *kapi.Secret) (*tls.Config, error) {
	peerCert, caCertPool, err := LoadCertsFromSecret(secret)
	if err != nil {
		return nil, err
	}

    return &tls.Config{
        Certificates: []tls.Certificate{*peerCert},
        RootCAs:      caCertPool,
    }, nil
}

// GetKubeClient can be used to obtain a pointer to k8s client
func GetKubeClient() (*kube.Kube, error) {
	clientset, err := kube.GetKubeConfig()
	if err != nil {
		return nil, err
	}

	kubecli := &kube.Kube{KClient: clientset}
	return kubecli, nil
}

func getCertClient(namespace string) (*cm.CertificateInterface, error) {
	kubecli, err := GetKubeClient()
	if err != nil {
		return nil, err
	}

	client, err := kubecli.GetCertManagerClient()
	if err != nil {
		return nil, err
	}

	certs := client.Certificates(namespace)
	return &certs, nil
}

// GetCert is used to get the certificate from specific namespace
func GetCert(namespace, name string) (*cmv1.Certificate, error) {	
	client, err := getCertClient(namespace)
	if err != nil {
		return nil, err
	}

	c, err := (*client).Get(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func newNodusCert(name, namespace, ipaddr string) *cmv1.Certificate {
	newCert := cmv1.Certificate{}
	newCert.APIVersion = "cert-manager.io/v1"
	newCert.Kind = "Certificate"
    newCert.Name = name
	newCert.Namespace = namespace
	newCert.Spec = cmv1.CertificateSpec {
		CommonName: "*.svc.cluster.local",
		IsCA: false,
		IssuerRef: cmmetav1.ObjectReference {
			Name: DefaultIssuer,
			Kind: "Issuer",
		},
		SecretName: DefaultCert,
		IPAddresses: []string{ipaddr},
	}

	return &newCert
}

func newCniCert(name, namespace, commonName string) *cmv1.Certificate {
	newCert := cmv1.Certificate{}
	newCert.APIVersion = "cert-manager.io/v1"
	newCert.Kind = "Certificate"
    newCert.Name = name
	newCert.Namespace = namespace
	newCert.Spec = cmv1.CertificateSpec {
		CommonName: commonName,
		IsCA: false,
		IssuerRef: cmmetav1.ObjectReference {
			Name: DefaultIssuer,
			Kind: "Issuer",
		},
		SecretName: DefaultCniCert,
		DNSNames: []string{commonName},
	}

	return &newCert
}

func applyCertAndWait(cert *cmv1.Certificate) (*cmv1.Certificate, *kapi.Secret, error) {
	client, err := getCertClient(cert.ObjectMeta.Namespace)
	if err != nil {
		return  nil, nil, err
	}

	cc, err := (*client).Create(context.TODO(), cert, v1.CreateOptions{})
	if err != nil {
		return  nil, nil, err
	}

	kubecli, err := GetKubeClient()
	if err != nil {
		return cc, nil, err
	}

	s, err := WaitForSecret(kubecli, cc.Namespace, cc.Spec.SecretName)
	if err != nil {
		return cc, nil, err
	}

	return cc, s, nil
}

// CreateNodusCert creates certificate for nodus to use
func CreateNodusCert(name, namespace, ipaddr string) (*cmv1.Certificate, *kapi.Secret, error) {
	cert := newNodusCert(name, namespace, ipaddr)
	return applyCertAndWait(cert)
}

// CreateCniCert creates certificate for cni to use
func CreateCniCert(name, namespace, commonName string) (*cmv1.Certificate, *kapi.Secret, error) {
	cert := newCniCert(name, namespace, commonName)
	return applyCertAndWait(cert)
}

// WaitForSecret waits for secret to be created
func WaitForSecret(kubecli *kube.Kube, namespace, name string) (*kapi.Secret, error) {
	var s *kapi.Secret
	var err error
	for i:= 0; i < maxTries; i++ {
		fmt.Println(i)
		s, err = kubecli.GetSecret(namespace, name)
		if err != nil || s == nil {
			if i >= maxTries - 1 {
				return nil, err
			}
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	return s, err
}
