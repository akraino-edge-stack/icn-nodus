package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
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
	// DefaultOVNCert is a name of the certificate and the secret used by OVN
	DefaultOVNCert     = "nodus-ovn-cert"
	// DefaultOvnCertDir is a directory where OVN certificates are stored
	DefaultOvnCertDir  = "/opt/ovn-certs"
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

	// OpenshiftNamespace is a namespace where OVN secret is available when deployed on Openshift
	OpenshiftNamespace = "openshift-ovn-kubernetes"
	// OpenshiftCAName is a name of OVN CA cert secret in Openshift deployment
	OpenshiftCAName = "ovn-ca"
	// OpenshiftCertName is a name of OVN cert secret in Openshift deployment
	OpenshiftCertName = "ovn-cert"
	// OpenshiftOVNSvc is a name of OVN service in Openshift deployment
	OpenshiftOVNSvc = "ovnkube-db"
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

// CreateServerTLSFromSecret creates TLS credentials for the GRPC server
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

// PrepareOVNSecrets gets and saves OVN related secrets on k8s/Openshift cluster if possible
func PrepareOVNSecrets(namespace string) (bool, error) {
	isOpenshift := false
	c, err := kube.GetKubeClient()
	if err != nil { 
		return isOpenshift, err
	}

	if err = os.MkdirAll(DefaultOvnCertDir, 0420); err != nil {
		return isOpenshift, err
	}

	s, err := c.GetSecret(namespace, DefaultOVNCert)

	if s == nil || err != nil {
		isOpenshift = true

		s, err = c.GetSecret(OpenshiftNamespace, OpenshiftCAName)
		if err != nil {
			return isOpenshift, err
		}

		ss, err := c.GetSecret(OpenshiftNamespace, OpenshiftCertName)
		if err != nil {
			return isOpenshift, err
		}

		if err = saveOpenshiftOVNSecret(s, ss, DefaultOvnCertDir); err != nil {
			return isOpenshift, err
		}
	} else {
		if err = saveKubernetesOVNSecret(s, DefaultOvnCertDir); err != nil {
			return isOpenshift, err
		}
	}
	return isOpenshift, nil
}

func getCertClient(namespace string) (*cm.CertificateInterface, error) {
	kubecli, err := kube.GetKubeClient()
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
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
		}
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*peerCert},
		RootCAs:      caCertPool,
		MinVersion: tls.VersionTLS12,
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

	kubecli, err := kube.GetKubeClient()
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

	kubecli, err := kube.GetKubeClient()
	if err != nil {
		return cc, nil, err
	}

	s, err := WaitForSecretIP(kubecli, cc)
	if err != nil {
		return cc, nil, err
	}

	return cc, s, nil
}

// saveOpenshiftOVNSecret saves secret to files in provided path when deployed on Openshift
func saveOpenshiftOVNSecret(caSecret, ovnSecret *kapi.Secret, path string) error {
	ca := caSecret.Data[CertFile]
	cert := ovnSecret.Data[CertFile]
	key := ovnSecret.Data[KeyFile]

	return saveFiles(ca, cert, key, path)
}

// saveKubernetesOVNSecret saves secret to files in provided path when deployed on Kubernetes
func saveKubernetesOVNSecret(secret *kapi.Secret, path string) error {
	ca := secret.Data[CAFile]
	cert := secret.Data[CertFile]
	key := secret.Data[KeyFile]

	return saveFiles(ca, cert, key, path)
}

func saveFiles(ca, cert, key []byte, path string) error {
	if err := saveFile(ca, path, CAFile); err != nil {
		return err
	}

	if err := saveFile(cert, path, CertFile); err != nil {
		return err
	}

	if err := saveFile(key, path, KeyFile); err != nil {
		return err
	}

	return nil
}

func saveFile(data []byte, path, filename string) error {
	f, err := os.Create(filepath.Join(path, filename))
	defer f.Close()
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	return nil
}
