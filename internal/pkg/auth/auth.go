package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"time"

	"google.golang.org/grpc/credentials"
	kapi "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cm "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
)

const (
	// DefaultIssuer is a default cert-manager issuer used by Nodus
	DefaultIssuer      = "nodus-issuer"
	// DefaultCert is a name of the certificate and the secret used by nfn-agent and nfn-operator
	DefaultCert        = "nodus-cert"
	// DefaultCniCert is a name of the certificate and the secret used by cniserver and cnishim
	DefaultCniCert     = "nodus-cni-cert"
	// DefaultOvnCertDir is a directory where OVN certificates are stored
	DefaultOvnCertDir = "/opt/ovn-certs"
	// NamespaceEnv is a name of env variable that holds namespace Nodus is deployed in
	NamespaceEnv       = "POD_NAMESPACE"
	// NfnOperatorHostEnv is a name of env variable that holds nfn-operator's service IP address
	NfnOperatorHostEnv = "NFN_OPERATOR_SERVICE_HOST"

	maxTries = 60
	// CAFile is a name of CA certificate file
	CAFile   = "ca.crt"
	// CertFile is a name of private certificate file
	CertFile = "tls.crt"
	// KeyFile is a name of private certificate key file
	KeyFile  = "tls.key"

	ipAddrSecretAnnotationKey = "cert-manager.io/ip-sans"
)

// LoadCertsFromSecret creates TLS certificate using provided secret
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

// CreateClientTLSFromSecret creates TLS credentials for the GRPC server
func CreateServerTLSFromSecret(secret *kapi.Secret) (*credentials.TransportCredentials, error) {
	return createTLS(secret, true)
}

// CreateClientTLSFromSecret creates TLS credentials for the GRPC client
func CreateClientTLSFromSecret(secret *kapi.Secret) (*credentials.TransportCredentials, error) {
	return createTLS(secret, false)
}

// CreateClientTLSConfig creates TLS config for the client
func CreateClientTLSConfig(secret *kapi.Secret) (*tls.Config, error) {
	peerCert, caCertPool, err := LoadCertsFromSecret(secret)
	if err != nil {
		return nil, err
	}

	return CreateTLSConfig(peerCert, caCertPool, false), nil
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

// UpdateCertIP updates IP that certificate is issued for
func UpdateCertIP(cert *cmv1.Certificate, ipAddr string) (*cmv1.Certificate, *kapi.Secret, error) {
	cert.Spec.IPAddresses = []string{ipAddr}
	return updateCertAndWait(cert)
}

// WaitForSecret waits for secret to be created
func WaitForSecret(kubecli *kube.Kube, namespace, name string) (*kapi.Secret, error) {
	var s *kapi.Secret
	var err error
	for i:= 0; i < maxTries; i++ {
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

// WaitForSecretIP waits for secret to have proper IP address
func WaitForSecretIP(kubecli *kube.Kube, cert *cmv1.Certificate) (*kapi.Secret, error) {
	var s *kapi.Secret
	var err error
	for i:= 0; i < maxTries; i++ {
		s, err = kubecli.GetSecret(cert.Namespace, cert.Spec.SecretName)
		if err != nil || s == nil {
			if i >= maxTries - 1 {
				return nil, err
			}
			time.Sleep(time.Second)
		} else {
			addresses := strings.Split(s.Annotations[ipAddrSecretAnnotationKey], ",")
			isOK := true
			for i, secAddr := range addresses {
				if secAddr != cert.Spec.IPAddresses[i] {
					isOK = false
					break
				}
			}
			if isOK {
				break
			} else {
				time.Sleep(time.Second)
			}
		}
	}
	return s, err
}

// IsCertIPUpToDate checks if the IP address that certificate was issued for is up to date
func IsCertIPUpToDate(crt *cmv1.Certificate, ipAddr string) bool {
	for _, ip := range crt.Spec.IPAddresses {
		if ip == ipAddr {
			return true
		}
	}
	return false
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

func createTLS(secret *kapi.Secret, isServer bool) (*credentials.TransportCredentials, error) {
	peerCert, caCertPool, err := LoadCertsFromSecret(secret)
	if err != nil {
		return nil, err
	}

	credTLS := credentials.NewTLS(CreateTLSConfig(peerCert, caCertPool, isServer))

	return &credTLS, nil
}

// CreateTLSConfig creats TLS config for server or client
func CreateTLSConfig(peerCert *tls.Certificate, caCertPool *x509.CertPool, isServer bool) *tls.Config {
	if isServer {
		return &tls.Config{
			Certificates: []tls.Certificate{*peerCert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS13,
			CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
		}
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*peerCert},
		RootCAs:      caCertPool,
		MinVersion: tls.VersionTLS13,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
	}
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

func updateCertAndWait(cert *cmv1.Certificate) (*cmv1.Certificate, *kapi.Secret, error) {
	client, err := getCertClient(cert.ObjectMeta.Namespace)
	if err != nil {
		return  nil, nil, err
	}

	cc, err := (*client).Update(context.TODO(), cert, v1.UpdateOptions{})
	if err != nil {
		return  nil, nil, err
	}

	kubecli, err := GetKubeClient()
	if err != nil {
		return cc, nil, err
	}

	s, err := WaitForSecretIP(kubecli, cc)
	if err != nil {
		return cc, nil, err
	}

	return cc, s, nil
}
