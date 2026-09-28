package keys

import (
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
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
