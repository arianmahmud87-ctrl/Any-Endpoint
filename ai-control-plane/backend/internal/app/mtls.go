package app

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func loadMTLSCA(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mTLS CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("mTLS CA contains no valid certificates")
	}
	return pool, nil
}

func serverMTLSConfig(cfg Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.MTLSServerCertFile, cfg.MTLSServerKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load mTLS server certificate: %w", err)
	}
	pool, err := loadMTLSCA(cfg.MTLSCAFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

func clientMTLSConfig(cfg Config, serverName string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.MTLSClientCertFile, cfg.MTLSClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load mTLS client certificate: %w", err)
	}
	pool, err := loadMTLSCA(cfg.MTLSCAFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   serverName,
	}, nil
}
