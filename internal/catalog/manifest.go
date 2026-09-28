package catalog

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"github.com/0xh4ty/quailfs/internal/backup"
	"github.com/0xh4ty/quailfs/internal/codec"
	"github.com/0xh4ty/quailfs/pkg/types"
)

func CreateManifest(datasetID []byte, generation uint64, parentManifestID []byte, trees []types.Tree, mchunks []types.MChunk, mstripes []types.MStripe, tombstones []string, catalogKey []byte, ed25519privateKey []byte) (types.ManifestEnvelope, error) {
	var manifest types.ManifestEnvelope

	manifest.ManifestPlain.DatasetID = datasetID
	manifest.ManifestPlain.Generation = generation
	manifest.ManifestPlain.ParentManifestID = parentManifestID
	manifest.ManifestPlain.Trees = trees
	manifest.ManifestPlain.MChunks = mchunks
	manifest.ManifestPlain.MStripes = mstripes
	manifest.ManifestPlain.Tombstones = tombstones

	serializedManifestPlain := backup.SerializeManifestPlain(manifest)

	envelope, err := codec.EncryptManifest(catalogKey, serializedManifestPlain)
	if err != nil {
		return types.ManifestEnvelope{}, err
	}
	manifest.Envelope = envelope

	var buf bytes.Buffer
	buf.WriteString("quailfs/manifest/v1")
	buf.Write(envelope)

	signature := ed25519.Sign(ed25519privateKey, buf.Bytes())

	manifest.Sig.Signature = signature

	var manifestIDBuf bytes.Buffer
	manifestIDBuf.Write(envelope)
	manifestIDBuf.Write(signature)

	manifestID := sha256.Sum256(manifestIDBuf.Bytes())
	manifest.ManifestID = manifestID[:]

	return manifest, nil
}

func OpenManifestEnvelope(manifest *types.ManifestEnvelope, catalogKey []byte) error {
	serializedManifestPlain, err := codec.DecryptManifest(
		catalogKey,
		manifest.Envelope,
	)
	if err != nil {
		return fmt.Errorf("decrypt manifest: %w", err)
	}

	manifestPlain, _, err := backup.DeserializeManifestPlain(serializedManifestPlain)
	if err != nil {
		return fmt.Errorf("deserialize manifest plain: %w", err)
	}

	manifest.ManifestPlain = manifestPlain

	return nil
}
