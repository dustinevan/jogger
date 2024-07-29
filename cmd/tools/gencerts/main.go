package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

var certDir = "certs/"

func main() {

	if _, err := os.Stat(certDir); os.IsNotExist(err) {
		os.Mkdir(certDir, 0755)
	}

	crt, key, certAbsPath := caCert()
	serverCertAbsPath, serverKeyAbsPath := serverCert(crt, key)
	clientCertAbsPath, clientKeyAbsPath := clientCert(crt, key)

	fmt.Println("Certificates generated successfully.")
	// Print exports needed for client and server
	fmt.Printf(`
    To use the generated certificates, set the following environment variables:
    For the server:
    
        export JOGGER_CA_CERT_FILE=%s
        export JOGGER_SERVER_PORT=%d
        export JOGGER_SERVER_CERT_FILE=%s
        export JOGGER_SERVER_KEY_FILE=%s
    
    For the client:
    
        export JOGGER_CA_CERT_FILE=%s
        export JOGGER_USER_CERT_FILE=%s
        export JOGGER_USER_KEY_FILE=%s
        export JOGGER_HOST=localhost:50051

`, certAbsPath, 50051, serverCertAbsPath, serverKeyAbsPath, certAbsPath, clientCertAbsPath, clientKeyAbsPath)
}

var maxInt128 = new(big.Int).Lsh(big.NewInt(1), 128)

func caCert() (cert *x509.Certificate, key *ecdsa.PrivateKey, certAbsPath string) {
	// Generate a ECDSA P256 key pair
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fmt.Printf("failed to generate ECDSA P256 key pair: %v\n", err)
		os.Exit(1)
	}

	// Generate a serial number for the certificate
	serialNumber, err := rand.Int(rand.Reader, maxInt128)
	if err != nil {
		fmt.Printf("failed to generate serial number: %v\n", err)
		os.Exit(1)
	}

	// A self-signed certificate must be marked as a CA, and have the digital signature and cert sign key usage bits set
	certTemplate := x509.Certificate{
		Subject:               pkix.Name{Organization: []string{"Jogger"}, CommonName: "localhost"},
		Issuer:                pkix.Name{Organization: []string{"Jogger"}, CommonName: "localhost"},
		SerialNumber:          serialNumber,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	// Create the self-signed CA certificate, the cert template is used as both the template and parent
	certBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &private.PublicKey, private)
	if err != nil {
		fmt.Printf("failed to create self-signed CA certificate: %v\n", err)
		os.Exit(1)
	}

	// This generation program creates client and server certs, so we need the CA cert and key later in the
	// process, we parse this so that we can return it.
	cert, err = x509.ParseCertificate(certBytes)
	if err != nil {
		fmt.Printf("failed to parse self-signed CA certificate: %v\n", err)
		os.Exit(1)
	}

	// Write the certificate and private key to files
	certFile, err := os.Create("certs/ca_tls.crt")
	if err != nil {
		fmt.Printf("failed to create cert file: %v\n", err)
		os.Exit(1)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		fmt.Printf("failed to write cert file: %v\n", err)
		os.Exit(1)
	}

	keyFile, err := os.Create("certs/ca_tls.key")
	if err != nil {
		fmt.Printf("failed to create key file: %v\n", err)
		os.Exit(1)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalECPrivateKey(private)
	if err != nil {
		fmt.Printf("failed to marshal private key: %v\n", err)
		os.Exit(1)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		fmt.Printf("failed to write key file: %v\n", err)
		os.Exit(1)
	}

	// Return the absolute path of the certificate file
	certAbsPath, err = filepath.Abs(certFile.Name())
	if err != nil {
		fmt.Printf("failed to get absolute path of cert file: %v\n", err)
		os.Exit(1)
	}
	return cert, private, certAbsPath
}

func serverCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (keyAbsPath string, certAbsPath string) {
	// Generate a ECDSA P256 key pair
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fmt.Printf("failed to generate ECDSA P256 key pair: %v\n", err)
		os.Exit(1)
	}

	// Generate a serial number for the certificate
	serialNumber, err := rand.Int(rand.Reader, maxInt128)
	if err != nil {
		fmt.Printf("failed to generate serial number: %v\n", err)
		os.Exit(1)
	}

	// Create a certificate template for the server
	certTemplate := x509.Certificate{
		Subject:               pkix.Name{Organization: []string{"Jogger"}, CommonName: "server1"},
		Issuer:                pkix.Name{Organization: []string{"Jogger"}, CommonName: "localhost"},
		SerialNumber:          serialNumber,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost:50051"},
	}

	// Create the server certificate using the CA certificate and private key
	certBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, caCert, &private.PublicKey, caKey)
	if err != nil {
		fmt.Printf("failed to create server certificate: %v\n", err)
		os.Exit(1)
	}

	// Write the certificate and private key to files
	certFile, err := os.Create("certs/server1_tls.crt")
	if err != nil {
		fmt.Printf("failed to create cert file: %v\n", err)
		os.Exit(1)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		fmt.Printf("failed to write cert file: %v\n", err)
		os.Exit(1)
	}

	keyFile, err := os.Create("certs/server1_tls.key")
	if err != nil {
		fmt.Printf("failed to create key file: %v\n", err)
		os.Exit(1)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalECPrivateKey(private)
	if err != nil {
		fmt.Printf("failed to marshal private key: %v\n", err)
		os.Exit(1)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		fmt.Printf("failed to write key file: %v\n", err)
		os.Exit(1)
	}
	// Return the absolute path of the certificate and key files
	certAbsPath, err = filepath.Abs(certFile.Name())
	if err != nil {
		fmt.Printf("failed to get absolute path of cert file: %v\n", err)
		os.Exit(1)
	}
	keyAbsPath, err = filepath.Abs(keyFile.Name())
	if err != nil {
		fmt.Printf("failed to get absolute path of key file: %v\n", err)
		os.Exit(1)
	}
	return certAbsPath, keyAbsPath
}

func clientCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (certAbsPath string, keyAbsPath string) {
	// Generate a ECDSA P256 key pair
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fmt.Printf("failed to generate ECDSA P256 key pair: %v\n", err)
		os.Exit(1)
	}

	// Generate a serial number for the certificate
	serialNumber, err := rand.Int(rand.Reader, maxInt128)
	if err != nil {
		fmt.Printf("failed to generate serial number: %v\n", err)
		os.Exit(1)
	}

	// Create a certificate template for the client
	certTemplate := x509.Certificate{
		Subject:               pkix.Name{Organization: []string{"Jogger"}, CommonName: "user1"},
		Issuer:                pkix.Name{Organization: []string{"Jogger"}, CommonName: "localhost"},
		SerialNumber:          serialNumber,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create the client certificate using the CA certificate and private key
	certBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, caCert, &private.PublicKey, caKey)
	if err != nil {
		fmt.Printf("failed to create client certificate: %v\n", err)
		os.Exit(1)
	}

	// Write the certificate and private key to files
	certFile, err := os.Create("certs/user1_tls.crt")
	if err != nil {
		fmt.Printf("failed to create cert file: %v\n", err)
		os.Exit(1)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		fmt.Printf("failed to write cert file: %v\n", err)
		os.Exit(1)
	}

	keyFile, err := os.Create("certs/user1_tls.key")
	if err != nil {
		fmt.Printf("failed to create key file: %v\n", err)
		os.Exit(1)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalECPrivateKey(private)
	if err != nil {
		fmt.Printf("failed to marshal private key: %v\n", err)
		os.Exit(1)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		fmt.Printf("failed to write key file: %v\n", err)
		os.Exit(1)
	}
	// Return the absolute path of the certificate and key files
	certAbsPath, err = filepath.Abs(certFile.Name())
	if err != nil {
		fmt.Printf("failed to get absolute path of cert file: %v\n", err)
		os.Exit(1)
	}
	keyAbsPath, err = filepath.Abs(keyFile.Name())
	if err != nil {
		fmt.Printf("failed to get absolute path of key file: %v\n", err)
		os.Exit(1)
	}
	return certAbsPath, keyAbsPath
}
