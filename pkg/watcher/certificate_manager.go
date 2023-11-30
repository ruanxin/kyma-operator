package watcher

import (
	"context"
	"fmt"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apicorev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kyma-project/lifecycle-manager/api/v1beta2"
	"github.com/kyma-project/lifecycle-manager/pkg/log"
	"github.com/kyma-project/lifecycle-manager/pkg/util"
)

const (
	// private key will only be generated if one does not already exist in the target `spec.secretName`.
	privateKeyRotationPolicy = "Never"

	DomainAnnotation = v1beta2.SKRDomainAnnotation

	caCertKey        = "ca.crt"
	tlsCertKey       = "tls.crt"
	tlsPrivateKeyKey = "tls.key"
)

//nolint:gochecknoglobals
var LabelSet = k8slabels.Set{
	v1beta2.PurposeLabel: v1beta2.CertManager,
	v1beta2.ManagedBy:    v1beta2.OperatorName,
}

type SubjectAltName struct {
	DNSNames       []string
	IPAddresses    []string
	URIs           []string
	EmailAddresses []string
}

type CertificateManager struct {
	kcpClient       client.Client
	caCertCache     *CACertificateCache
	certificateName string
	secretName      string
	config          CertificateConfig
}

type CertificateConfig struct {
	// IstioNamespace represents the cluster resource namespace of istio
	IstioNamespace string
	// RemoteSyncNamespace indicates the sync namespace for Kyma and module catalog
	RemoteSyncNamespace string
	// CACertificateName indicates the Name of the CA Root Certificate in the Istio Namespace
	CACertificateName string
	// AdditionalDNSNames indicates the DNS Names which should be added additional to the Subject
	// Alternative Names of each Kyma Certificate
	AdditionalDNSNames []string
	Duration           apimetav1.Duration
	RenewBefore        apimetav1.Duration
	RenewBuffer        time.Duration
}

type CertificateSecret struct {
	CACrt           string
	TLSCrt          string
	TLSKey          string
	ResourceVersion string
}

// NewCertificateManager returns a new CertificateManager, which can be used for creating a cert-manager Certificates.
func NewCertificateManager(kcpClient client.Client, kymaName string,
	config CertificateConfig,
	caCertCache *CACertificateCache,
) *CertificateManager {
	return &CertificateManager{
		kcpClient:       kcpClient,
		certificateName: ResolveTLSCertName(kymaName),
		secretName:      ResolveTLSCertName(kymaName),
		config:          config,
		caCertCache:     caCertCache,
	}
}

// CreateSelfSignedCert creates a cert-manager Certificate with a sufficient set of Subject-Alternative-Names.
func (c *CertificateManager) CreateSelfSignedCert(ctx context.Context, kyma *v1beta2.Kyma) (*certmanagerv1.Certificate,
	error,
) {
	subjectAltNames, err := c.getSubjectAltNames(kyma)
	if err != nil {
		return nil, fmt.Errorf("error get Subject Alternative Name from KymaCR: %w", err)
	}
	return c.patchCertificate(ctx, subjectAltNames)
}

// Remove removes the certificate including its certificate secret.
func (c *CertificateManager) Remove(ctx context.Context) error {
	err := c.RemoveCertificate(ctx)
	if err != nil {
		return err
	}

	return c.RemoveSecret(ctx)
}

func (c *CertificateManager) RemoveCertificate(ctx context.Context) error {
	certificate := &certmanagerv1.Certificate{}
	if err := c.kcpClient.Get(ctx, client.ObjectKey{
		Name:      c.certificateName,
		Namespace: c.config.IstioNamespace,
	}, certificate); err != nil && !util.IsNotFound(err) {
		return fmt.Errorf("failed to get certificate: %w", err)
	}

	if err := c.kcpClient.Delete(ctx, certificate); err != nil {
		return fmt.Errorf("failed to delete certificate: %w", err)
	}

	return nil
}

func (c *CertificateManager) RemoveSecret(ctx context.Context) error {
	certSecret := &apicorev1.Secret{}
	if err := c.kcpClient.Get(ctx, client.ObjectKey{
		Name:      c.secretName,
		Namespace: c.config.IstioNamespace,
	}, certSecret); err != nil && !util.IsNotFound(err) {
		return fmt.Errorf("failed to get certificate secret: %w", err)
	}

	err := c.kcpClient.Delete(ctx, certSecret)
	if err != nil {
		return fmt.Errorf("failed to delete certificate secret: %w", err)
	}
	return nil
}

func (c *CertificateManager) GetSecret(ctx context.Context) (*CertificateSecret, error) {
	secret := &apicorev1.Secret{}
	err := c.kcpClient.Get(ctx, client.ObjectKey{Name: c.secretName, Namespace: c.config.IstioNamespace},
		secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret for certificate %s-%s: %w", c.secretName, c.config.IstioNamespace,
			err)
	}
	certSecret := CertificateSecret{
		CACrt:           string(secret.Data[caCertKey]),
		TLSCrt:          string(secret.Data[tlsCertKey]),
		TLSKey:          string(secret.Data[tlsPrivateKeyKey]),
		ResourceVersion: secret.GetResourceVersion(),
	}
	return &certSecret, nil
}

func (c *CertificateManager) patchCertificate(ctx context.Context,
	subjectAltName *SubjectAltName,
) (*certmanagerv1.Certificate, error) {
	issuer, err := c.getIssuer(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting issuer: %w", err)
	}

	cert := certmanagerv1.Certificate{
		TypeMeta: apimetav1.TypeMeta{
			Kind:       certmanagerv1.CertificateKind,
			APIVersion: certmanagerv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: apimetav1.ObjectMeta{
			Name:      c.certificateName,
			Namespace: c.config.IstioNamespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			Duration:       &c.config.Duration,
			RenewBefore:    &c.config.RenewBefore,
			DNSNames:       subjectAltName.DNSNames,
			IPAddresses:    subjectAltName.IPAddresses,
			URIs:           subjectAltName.URIs,
			EmailAddresses: subjectAltName.EmailAddresses,
			SecretName:     c.secretName,
			SecretTemplate: &certmanagerv1.CertificateSecretTemplate{
				Labels: LabelSet,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: issuer.Name,
			},
			IsCA: false,
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageDigitalSignature,
				certmanagerv1.UsageKeyEncipherment,
			},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				RotationPolicy: privateKeyRotationPolicy,
				Encoding:       certmanagerv1.PKCS1,
				Algorithm:      certmanagerv1.RSAKeyAlgorithm,
			},
		},
	}

	err = c.kcpClient.Patch(ctx, &cert, client.Apply, client.ForceOwnership, skrChartFieldOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to patch certificate: %w", err)
	}
	return &cert, nil
}

func (c *CertificateManager) getSubjectAltNames(kyma *v1beta2.Kyma) (*SubjectAltName, error) {
	if domain, ok := kyma.Annotations[DomainAnnotation]; ok {
		if domain == "" {
			return nil, fmt.Errorf("Domain-Annotation of KymaCR %s is empty", kyma.Name) //nolint:goerr113
		}

		svcSuffix := []string{"svc.cluster.local", "svc"}
		dnsNames := []string{domain}

		for _, suffix := range svcSuffix {
			dnsNames = append(dnsNames, fmt.Sprintf("%s.%s.%s", SkrResourceName, c.config.RemoteSyncNamespace, suffix))
		}

		dnsNames = append(dnsNames, c.config.AdditionalDNSNames...)

		return &SubjectAltName{
			DNSNames: dnsNames,
		}, nil
	}
	return nil, fmt.Errorf("kymaCR %s does not contain annotation '%s' with specified domain", //nolint:goerr113
		kyma.Name, DomainAnnotation)
}

func (c *CertificateManager) getIssuer(ctx context.Context) (*certmanagerv1.Issuer, error) {
	logger := logf.FromContext(ctx)
	issuerList := &certmanagerv1.IssuerList{}
	err := c.kcpClient.List(ctx, issuerList, &client.ListOptions{
		LabelSelector: k8slabels.SelectorFromSet(LabelSet),
		Namespace:     c.config.IstioNamespace,
	})
	if err != nil {
		return nil, fmt.Errorf("could not list cert-manager issuer %w", err)
	}
	if len(issuerList.Items) == 0 {
		return nil, fmt.Errorf("no issuer found in Namespace `%s`", c.config.IstioNamespace) //nolint:goerr113
	} else if len(issuerList.Items) > 1 {
		logger.Info("Found more than one issuer, will use by default first one in list",
			"issuer", issuerList.Items)
	}
	return &issuerList.Items[0], nil
}

func (c *CertificateManager) GetCertificateSecret(ctx context.Context) (*apicorev1.Secret, error) {
	secret := &apicorev1.Secret{}
	err := c.kcpClient.Get(ctx, client.ObjectKey{Name: c.secretName, Namespace: c.config.IstioNamespace},
		secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret for certificate %s-%s: %w", c.secretName, c.config.IstioNamespace,
			err)
	}

	return secret, nil
}

type CertificateNotReadyError struct{}

func (e *CertificateNotReadyError) Error() string {
	return "Certificate-Secret does not exist"
}

func (c *CertificateManager) GetCACertificate(ctx context.Context) (*certmanagerv1.Certificate, error) {
	logger := logf.FromContext(ctx)
	cachedCert := c.caCertCache.GetCACertFromCache(c.config.CACertificateName)

	if cachedCert == nil || certificateRenewalTimePassed(cachedCert) {
		caCert := &certmanagerv1.Certificate{}
		if err := c.kcpClient.Get(ctx,
			client.ObjectKey{Namespace: c.config.IstioNamespace, Name: c.config.CACertificateName},
			caCert); err != nil {
			return nil, fmt.Errorf("failed to get CA certificate %w", err)
		}
		c.caCertCache.SetCACertToCache(caCert)
		logger.V(log.InfoLevel).Info("get caCert from cluster")
		return caCert, nil
	}
	logger.V(log.InfoLevel).Info("get caCert from cache")
	return cachedCert, nil
}

func certificateRenewalTimePassed(cert *certmanagerv1.Certificate) bool {
	return cert.Status.RenewalTime.Before(&(apimetav1.Time{Time: time.Now()}))
}
