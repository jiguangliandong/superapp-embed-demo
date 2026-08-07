package server

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
)

var (
	oidECPublicKey = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	oidPrimeField  = asn1.ObjectIdentifier{1, 2, 840, 10045, 1, 1}
)

type pkcs8PrivateKey struct {
	Version    int
	Algorithm  pkix.AlgorithmIdentifier
	PrivateKey []byte
}

type explicitFieldID struct {
	FieldType asn1.ObjectIdentifier
	Prime     *big.Int
}

type explicitCurve struct {
	A    []byte
	B    []byte
	Seed asn1.BitString `asn1:"optional"`
}

type explicitECParameters struct {
	Version  int
	FieldID  explicitFieldID
	Curve    explicitCurve
	Base     []byte
	Order    *big.Int
	Cofactor *big.Int `asn1:"optional"`
}

type sec1PrivateKey struct {
	Version    int
	PrivateKey []byte
	Parameters asn1.RawValue  `asn1:"optional,explicit,tag:0"`
	PublicKey  asn1.BitString `asn1:"optional,explicit,tag:1"`
}

func loadPrivateKey(path string) (crypto.Signer, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Partner private key: %w", err)
	}
	block, _ := pem.Decode(contents)
	if block == nil {
		return nil, errors.New("Partner private key is not PEM encoded")
	}

	var parsed any
	parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if ecKey, ecErr := x509.ParseECPrivateKey(block.Bytes); ecErr == nil {
			parsed = ecKey
			err = nil
		}
	}
	if err != nil && block.Type == "PRIVATE KEY" {
		if ecKey, explicitErr := parseExplicitP256PKCS8(block.Bytes); explicitErr == nil {
			parsed = ecKey
			err = nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("parse Partner private key: %w", err)
	}

	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("Partner private key must be an ES256 P-256 key")
	}
	return key, nil
}

func parseExplicitP256PKCS8(der []byte) (*ecdsa.PrivateKey, error) {
	var outer pkcs8PrivateKey
	rest, err := asn1.Unmarshal(der, &outer)
	if err != nil || len(rest) != 0 || outer.Version != 0 ||
		!outer.Algorithm.Algorithm.Equal(oidECPublicKey) {
		return nil, errors.New("private key is not an EC PKCS#8 key")
	}

	var parameters explicitECParameters
	rest, err = asn1.Unmarshal(outer.Algorithm.Parameters.FullBytes, &parameters)
	if err != nil || len(rest) != 0 || !isP256Parameters(parameters) {
		return nil, errors.New("explicit EC parameters are not standard P-256")
	}

	var inner sec1PrivateKey
	rest, err = asn1.Unmarshal(outer.PrivateKey, &inner)
	if err != nil || len(rest) != 0 || inner.Version != 1 {
		return nil, errors.New("invalid SEC1 private key")
	}
	curve := elliptic.P256()
	d := new(big.Int).SetBytes(inner.PrivateKey)
	if d.Sign() <= 0 || d.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("P-256 private scalar is out of range")
	}
	x, y := curve.ScalarBaseMult(inner.PrivateKey)
	if inner.PublicKey.BitLength > 0 {
		publicX, publicY := elliptic.Unmarshal(curve, inner.PublicKey.Bytes)
		if publicX == nil || publicX.Cmp(x) != 0 || publicY.Cmp(y) != 0 {
			return nil, errors.New("embedded EC public key does not match private key")
		}
	}
	return &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y},
		D:         d,
	}, nil
}

func isP256Parameters(parameters explicitECParameters) bool {
	curve := elliptic.P256()
	values := curve.Params()
	if parameters.Version != 1 ||
		!parameters.FieldID.FieldType.Equal(oidPrimeField) ||
		parameters.FieldID.Prime == nil || parameters.FieldID.Prime.Cmp(values.P) != 0 ||
		parameters.Order == nil || parameters.Order.Cmp(values.N) != 0 ||
		parameters.Cofactor == nil || parameters.Cofactor.Cmp(big.NewInt(1)) != 0 {
		return false
	}
	expectedA := new(big.Int).Sub(values.P, big.NewInt(3))
	if new(big.Int).SetBytes(parameters.Curve.A).Cmp(expectedA) != 0 ||
		new(big.Int).SetBytes(parameters.Curve.B).Cmp(values.B) != 0 {
		return false
	}
	expectedBase := elliptic.Marshal(curve, values.Gx, values.Gy)
	return bytes.Equal(parameters.Base, expectedBase)
}
