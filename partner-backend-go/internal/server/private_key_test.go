package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrivateKeyAcceptsExplicitStandardP256Parameters(t *testing.T) {
	original, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der := marshalExplicitP256PKCS8(t, original)
	path := filepath.Join(t.TempDir(), "partner.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY", Bytes: der,
	}), 0o600); err != nil {
		t.Fatalf("write test key: %v", err)
	}

	loaded, err := loadPrivateKey(path)
	if err != nil {
		t.Fatalf("load explicit P-256 key: %v", err)
	}
	key := loaded.(*ecdsa.PrivateKey)
	if key.D.Cmp(original.D) != 0 ||
		key.PublicKey.X.Cmp(original.PublicKey.X) != 0 ||
		key.PublicKey.Y.Cmp(original.PublicKey.Y) != 0 {
		t.Fatal("loaded key does not match original key material")
	}
}

func marshalExplicitP256PKCS8(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	curve := elliptic.P256()
	values := curve.Params()
	parametersDER, err := asn1.Marshal(explicitECParameters{
		Version: 1,
		FieldID: explicitFieldID{
			FieldType: oidPrimeField,
			Prime:     values.P,
		},
		Curve: explicitCurve{
			A: new(big.Int).Sub(values.P, big.NewInt(3)).FillBytes(make([]byte, 32)),
			B: values.B.FillBytes(make([]byte, 32)),
		},
		Base:     elliptic.Marshal(curve, values.Gx, values.Gy),
		Order:    values.N,
		Cofactor: big.NewInt(1),
	})
	if err != nil {
		t.Fatalf("marshal explicit parameters: %v", err)
	}
	innerDER, err := asn1.Marshal(sec1PrivateKey{
		Version:    1,
		PrivateKey: key.D.FillBytes(make([]byte, 32)),
		PublicKey: asn1.BitString{
			Bytes:     elliptic.Marshal(curve, key.PublicKey.X, key.PublicKey.Y),
			BitLength: 65 * 8,
		},
	})
	if err != nil {
		t.Fatalf("marshal SEC1 key: %v", err)
	}
	outerDER, err := asn1.Marshal(pkcs8PrivateKey{
		Version: 0,
		Algorithm: pkix.AlgorithmIdentifier{
			Algorithm: oidECPublicKey,
			Parameters: asn1.RawValue{
				FullBytes: parametersDER,
			},
		},
		PrivateKey: innerDER,
	})
	if err != nil {
		t.Fatalf("marshal PKCS#8 key: %v", err)
	}
	return outerDER
}
