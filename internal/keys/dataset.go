package keys

import (
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"github.com/0xh4ty/quailfs/pkg/types"
	"golang.org/x/crypto/chacha20poly1305"
)

func GenerateDatasetKey() ([]byte, error) {
	datasetKey := make([]byte, 32)
	_, err := rand.Read(datasetKey)
	if err != nil {
		return nil, err
	}
	return datasetKey, nil
}

func DeriveDatasetID(userID []byte, label string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("quailfs/dataset/v1"))
	buf.Write(userID)
	buf.Write([]byte(label))
	datasetID := sha256.Sum256(buf.Bytes())
	return datasetID[:]
}

func DeriveCatalogKey(datasetKey []byte, datasetID []byte) ([]byte, error) {
	catalogKey, err := hkdf.Key(sha256.New, datasetKey, datasetID, "quailfs/catalog/v1", 32)
	if err != nil {
		return nil, err
	}
	return catalogKey, nil
}

func DeriveDataKey(datasetKey []byte, datasetID []byte) ([]byte, error) {
	dataKey, err := hkdf.Key(sha256.New, datasetKey, datasetID, "quailfs/data/v1", 32)
	if err != nil {
		return nil, err
	}
	return dataKey, nil
}

func DeriveNameKey(datasetKey []byte, datasetID []byte) ([]byte, error) {
	nameKey, err := hkdf.Key(sha256.New, datasetKey, datasetID, "quailfs/name/v1", 32)
	if err != nil {
		return nil, err
	}
	return nameKey, nil
}

func DeriveUserIndexKey(userID []byte) []byte {
	hash := sha256.New()
	hash.Write([]byte("quailfs/userindex/v1"))
	hash.Write(userID)
	return hash.Sum(nil)
}

func DeriveHeadKey(userID []byte, datasetID []byte) []byte {
	hash := sha256.New()
	hash.Write([]byte("quailfs/headkey/v1"))
	hash.Write(userID)
	hash.Write(datasetID)
	return hash.Sum(nil)
}

func UnwrapDatasetKey(wrappedDataset types.WrappedDataset, x25519PrivateKey []byte) ([]byte, error) {
	ephemeralPublicKey := wrappedDataset.Body.EphX25519Pubkey

	sharedSecret, err := ComputeSharedSecret(
		x25519PrivateKey,
		ephemeralPublicKey,
	)
	if err != nil {
		return nil, err
	}

	aead, err := chacha20poly1305.NewX(sharedSecret)
	if err != nil {
		return nil, err
	}

	encryptedDatasetKey := wrappedDataset.Body.EncDatasetKey

	if len(encryptedDatasetKey) < aead.NonceSize() {
		return nil, fmt.Errorf("encrypted dataset key is too short")
	}

	nonce := encryptedDatasetKey[:aead.NonceSize()]
	ciphertext := encryptedDatasetKey[aead.NonceSize():]

	datasetKey, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt dataset key: %w", err)
	}

	return datasetKey, nil
}
