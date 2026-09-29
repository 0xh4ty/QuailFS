package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/0xh4ty/quailfs/internal/backup"
	"github.com/0xh4ty/quailfs/internal/codec"
	"github.com/0xh4ty/quailfs/internal/keys"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRoundTrip(t *testing.T) {
	// ------------- Setup ---------------

	mnemonic_passphrase := "fresh year gaze process tag evidence feature brown involve morning ripple taste secret mesh slot morning script father divorce box maximum language inject olympic"
	mnemonic_seed := keys.DeriveSeed(mnemonic_passphrase)

	ed25519Seed, _ := keys.DeriveEd25519Seed(mnemonic_seed)

	_, ed25519publicKey := keys.DeriveEd25519Keypair(ed25519Seed)
	datasetKey, _ := keys.GenerateDatasetKey()

	user_id := keys.DeriveUserID(ed25519publicKey)
	dataset_id := keys.DeriveDatasetID(user_id, "default")

	// ------------- Backup --------------

	homeDir, _ := os.UserHomeDir()
	file := filepath.Join(homeDir, ".quailfs-data", "test_file.txt")

	data_first, _ := os.ReadFile(file)

	hash_first := sha256.Sum256(data_first)
	hash_first_hex := hex.EncodeToString(hash_first[:])

	qchunks, _ := backup.Chunker(data_first)

	compressed_qchunks, _ := codec.Compressor(qchunks)

	packed_stripe_plain := backup.PackChunks(compressed_qchunks)

	packed_stripe_serialized := backup.SerializePackedPlainCollection(packed_stripe_plain)

	var stripe_ids [][]byte
	var shard_collection [][][]byte

	for i := range packed_stripe_serialized {
		stripe := packed_stripe_serialized[i]

		stripe_id := keys.DeriveStripeID(dataset_id, stripe)
		stripe_key, _ := keys.DeriveStripeKey(datasetKey, dataset_id, stripe_id)
		nonce_96, _ := keys.DeriveNonce96(stripe_key, dataset_id, stripe_id)

		encrypted_stripe, err := codec.EncryptStripe(stripe_key, nonce_96, stripe)
		if err != nil {
			t.Fatal(err)
		}

		stripe_ids = append(stripe_ids, stripe_id)

		shards, _ := codec.EncodeStripe(encrypted_stripe)

		shard_collection = append(shard_collection, shards)
	}

	// ------------- Restore --------------

	for i := range shard_collection {
		shards := shard_collection[i]

		encrypted_stripe, _ := codec.DecodeStripe(shards)

		stripe_id := stripe_ids[i]
		stripe_key, _ := keys.DeriveStripeKey(datasetKey, dataset_id, stripe_id)
		nonce_96, _ := keys.DeriveNonce96(stripe_key, dataset_id, stripe_id)

		packed_stripe_serialized[i], _ = codec.DecryptStripe(stripe_key, nonce_96, encrypted_stripe)
	}

	packed_stripe_plain = backup.DeserializePackedPlainCollection(packed_stripe_serialized)

	compressed_qchunks = backup.UnpackChunks((packed_stripe_plain))

	qchunks, _ = codec.Decompressor(compressed_qchunks)
	data_last, _ := backup.Dechunker(qchunks)

	hash_last := sha256.Sum256(data_last)
	hash_last_hex := hex.EncodeToString(hash_last[:])

	if hash_first_hex != hash_last_hex {
		t.Errorf("Data restore failed")
	}
}

func TestBackupRoundTripWithShardLoss(t *testing.T) {
	// ------------- Setup ---------------

	mnemonic_passphrase := "fresh year gaze process tag evidence feature brown involve morning ripple taste secret mesh slot morning script father divorce box maximum language inject olympic"
	mnemonic_seed := keys.DeriveSeed(mnemonic_passphrase)

	ed25519Seed, _ := keys.DeriveEd25519Seed(mnemonic_seed)

	_, ed25519publicKey := keys.DeriveEd25519Keypair(ed25519Seed)
	datasetKey, _ := keys.GenerateDatasetKey()

	user_id := keys.DeriveUserID(ed25519publicKey)
	dataset_id := keys.DeriveDatasetID(user_id, "default")

	// ------------- Backup --------------

	homeDir, _ := os.UserHomeDir()
	file := filepath.Join(homeDir, ".quailfs-data", "test_file.txt")

	data_first, _ := os.ReadFile(file)

	hash_first := sha256.Sum256(data_first)
	hash_first_hex := hex.EncodeToString(hash_first[:])

	qchunks, _ := backup.Chunker(data_first)

	compressed_qchunks, _ := codec.Compressor(qchunks)

	packed_stripe_plain := backup.PackChunks(compressed_qchunks)

	packed_stripe_serialized := backup.SerializePackedPlainCollection(packed_stripe_plain)

	var stripe_ids [][]byte
	var shard_collection [][][]byte

	for i := range packed_stripe_serialized {
		stripe := packed_stripe_serialized[i]

		stripe_id := keys.DeriveStripeID(dataset_id, stripe)
		stripe_key, _ := keys.DeriveStripeKey(datasetKey, dataset_id, stripe_id)
		nonce_96, _ := keys.DeriveNonce96(stripe_key, dataset_id, stripe_id)

		encrypted_stripe, err := codec.EncryptStripe(stripe_key, nonce_96, stripe)
		if err != nil {
			t.Fatal(err)
		}

		stripe_ids = append(stripe_ids, stripe_id)

		shards, _ := codec.EncodeStripe(encrypted_stripe)

		shard_collection = append(shard_collection, shards)
	}

	// ----------- Shard Loss -------------

	for i := range shard_collection {
		lost := make(map[int]bool)

		for len(lost) < 4 {
			index := rand.Intn(len(shard_collection[i]))
			if lost[index] {
				continue
			}

			lost[index] = true
			shard_collection[i][index] = nil
		}
	}

	// ------------- Restore --------------

	for i := range shard_collection {
		shards := shard_collection[i]

		encrypted_stripe, _ := codec.DecodeStripe(shards)

		stripe_id := stripe_ids[i]
		stripe_key, _ := keys.DeriveStripeKey(datasetKey, dataset_id, stripe_id)
		nonce_96, _ := keys.DeriveNonce96(stripe_key, dataset_id, stripe_id)

		packed_stripe_serialized[i], _ = codec.DecryptStripe(stripe_key, nonce_96, encrypted_stripe)
	}

	packed_stripe_plain = backup.DeserializePackedPlainCollection(packed_stripe_serialized)

	compressed_qchunks = backup.UnpackChunks((packed_stripe_plain))

	qchunks, _ = codec.Decompressor(compressed_qchunks)
	data_last, _ := backup.Dechunker(qchunks)

	hash_last := sha256.Sum256(data_last)
	hash_last_hex := hex.EncodeToString(hash_last[:])

	if hash_first_hex != hash_last_hex {
		t.Errorf("Data restore failed")
	}
}
