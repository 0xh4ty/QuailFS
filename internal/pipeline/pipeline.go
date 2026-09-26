package pipeline

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/0xh4ty/quailfs/internal/backup"
	"github.com/0xh4ty/quailfs/internal/catalog"
	"github.com/0xh4ty/quailfs/internal/codec"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/0xh4ty/quailfs/internal/network"
	"github.com/0xh4ty/quailfs/internal/place"
	"github.com/0xh4ty/quailfs/pkg/types"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"os"
	"path/filepath"
)

type BackupResult struct {
	Manifest        types.ManifestEnvelope
	Head            types.Head
	WrappedDataset  types.WrappedDataset
	UserIndex       types.UserIndex
	StripeIDs       [][]byte
	ShardCollection [][][]byte
}

type backupShards struct {
	stripeIDs [][]byte
	shards    [][][]byte
}

type catalogObject struct {
	name       string
	objectType uint8
	data       []byte
}

func backupFile(path string, datasetID []byte, datasetKey []byte) (types.File, []types.MChunk, []types.MStripe, backupShards, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return types.File{}, nil, nil, backupShards{}, fmt.Errorf("read %q: %w", path, err)
	}

	qchunks, err := backup.Chunker(data)
	if err != nil {
		return types.File{}, nil, nil, backupShards{}, fmt.Errorf("chunk %q: %w", path, err)
	}

	compressedQChunks, err := codec.Compressor(qchunks)
	if err != nil {
		return types.File{}, nil, nil, backupShards{}, fmt.Errorf("compress %q: %w", path, err)
	}

	packedStripePlain := backup.PackChunks(compressedQChunks)
	packedStripeSerialized := backup.SerializePackedPlainCollection(packedStripePlain)

	file := types.File{
		FileName: filepath.Base(path),
	}

	var mChunks []types.MChunk
	var mStripes []types.MStripe
	var stripeIDs [][]byte
	var shardCollection [][][]byte

	for _, qchunk := range qchunks {
		mChunks = append(mChunks, types.MChunk{
			ChunkID:  qchunk.ChunkID,
			Size:     uint64(len(qchunk.Data)),
			StripeID: nil,
		})

		file.ChunkIDs = append(file.ChunkIDs, qchunk.ChunkID)
	}

	for _, stripe := range packedStripeSerialized {
		stripeID := keys.DeriveStripeID(datasetID, stripe)

		stripeKey, err := keys.DeriveStripeKey(datasetKey, datasetID, stripeID)
		if err != nil {
			return types.File{}, nil, nil, backupShards{}, fmt.Errorf("derive stripe key: %w", err)
		}

		nonce96, err := keys.DeriveNonce96(stripeKey, datasetID, stripeID)
		if err != nil {
			return types.File{}, nil, nil, backupShards{}, fmt.Errorf("derive nonce: %w", err)
		}

		encryptedStripe, err := codec.EncryptStripe(stripeKey, nonce96, stripe)
		if err != nil {
			return types.File{}, nil, nil, backupShards{}, fmt.Errorf("encrypt stripe: %w", err)
		}

		shards, err := codec.EncodeStripe(encryptedStripe)
		if err != nil {
			return types.File{}, nil, nil, backupShards{}, fmt.Errorf("encode stripe: %w", err)
		}

		shardNames := make([][]byte, len(shards))

		for i := range shards {
			shardName := fmt.Sprintf("%x%02X", stripeID, i)
			shardNames[i] = []byte(shardName)
		}

		stripeIDs = append(stripeIDs, stripeID)
		shardCollection = append(shardCollection, shards)

		mStripes = append(mStripes, types.MStripe{
			StripeID:   stripeID,
			K:          8,
			N:          12,
			PayloadLen: uint64(len(encryptedStripe)),
			ShardNames: shardNames,
		})
	}

	return file, mChunks, mStripes, backupShards{
		stripeIDs: stripeIDs,
		shards:    shardCollection,
	}, nil
}

func deriveUserIndexKey(userID []byte) []byte {
	hash := sha256.New()
	hash.Write([]byte("quailfs/userindex/v1"))
	hash.Write(userID)
	return hash.Sum(nil)
}

func deriveHeadKey(userID []byte, datasetID []byte) []byte {
	hash := sha256.New()
	hash.Write([]byte("quailfs/headkey/v1"))
	hash.Write(userID)
	hash.Write(datasetID)
	return hash.Sum(nil)
}

func randomCatalogSuffix() (string, error) {
	buf := make([]byte, 16)

	if _, err := crand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

func Backup(ctx context.Context, paths []string, userID []byte, datasetID []byte, datasetKey []byte, catalogKey []byte, label string, generation uint64, parentManifestID []byte, userX25519Pubkey []byte, ed25519PrivateKey []byte, ed25519PublicKey []byte, kad *dht.IpfsDHT) error {
	var stripeIDs [][]byte
	var shardCollection [][][]byte

	var trees []types.Tree
	var mChunks []types.MChunk
	var mStripes []types.MStripe

	for _, rootPath := range paths {
		info, err := os.Stat(rootPath)
		if err != nil {
			return fmt.Errorf("stat %q: %w", rootPath, err)
		}

		tree := types.Tree{
			RootDirectory: filepath.Base(rootPath),
		}

		if info.IsDir() {
			err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if info.IsDir() {
					return nil
				}

				file, chunks, stripes, shards, err := backupFile(path, datasetID, datasetKey)
				if err != nil {
					return err
				}

				tree.Files = append(tree.Files, file)
				mChunks = append(mChunks, chunks...)
				mStripes = append(mStripes, stripes...)
				stripeIDs = append(stripeIDs, shards.stripeIDs...)
				shardCollection = append(shardCollection, shards.shards...)

				return nil
			})
			if err != nil {
				return fmt.Errorf("backup directory %q: %w", rootPath, err)
			}
		} else {
			file, chunks, stripes, shards, err := backupFile(rootPath, datasetID, datasetKey)
			if err != nil {
				return err
			}

			tree.Files = append(tree.Files, file)
			mChunks = append(mChunks, chunks...)
			mStripes = append(mStripes, stripes...)
			stripeIDs = append(stripeIDs, shards.stripeIDs...)
			shardCollection = append(shardCollection, shards.shards...)
		}

		trees = append(trees, tree)
	}

	manifest, err := catalog.CreateManifest(
		datasetID,
		generation,
		parentManifestID,
		trees,
		mChunks,
		mStripes,
		nil,
		catalogKey,
		ed25519PrivateKey,
	)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}

	wrappedDataset, err := catalog.CreateWrappedDataset(
		userID,
		datasetID,
		datasetKey,
		label,
		generation,
		userX25519Pubkey,
		ed25519PrivateKey,
	)
	if err != nil {
		return fmt.Errorf("create wrapped dataset: %w", err)
	}

	head := catalog.CreateHead(
		1,
		userID,
		datasetID,
		generation,
		manifest.ManifestID,
		nil,
		ed25519PrivateKey,
	)

	datasetEntry := types.DatasetEntry{
		DatasetID:  datasetID,
		Label:      label,
		Generation: generation,
	}

	userIndex := catalog.CreateUserIndex(
		1,
		userID,
		generation,
		[]types.DatasetEntry{datasetEntry},
		ed25519PrivateKey,
	)

	if kad == nil {
		return fmt.Errorf("DHT is not initialized")
	}

	h := kad.Host()

	livePeers := make([]peer.ID, 0)

	for _, p := range kad.RoutingTable().ListPeers() {
		if p == h.ID() {
			continue
		}

		livePeers = append(livePeers, p)
	}

	if len(livePeers) == 0 {
		return fmt.Errorf("no peers discovered through DHT")
	}

	var objectNames []string

	for _, stripes := range mStripes {
		for _, shardName := range stripes.ShardNames {
			objectNames = append(objectNames, string(shardName))
		}
	}

	placements := place.Place(livePeers, objectNames)

	for peerID, shardNames := range placements {
		for _, shardName := range shardNames {
			var shard []byte

			for stripeIndex := range mStripes {
				for shardIndex := range mStripes[stripeIndex].ShardNames {
					if string(mStripes[stripeIndex].ShardNames[shardIndex]) != shardName {
						continue
					}

					shard = shardCollection[stripeIndex][shardIndex]
					break
				}

				if shard != nil {
					break
				}
			}

			if shard == nil {
				return fmt.Errorf("shard %q not found", shardName)
			}

			if err := network.PutShard(ctx, h, peerID, shardName, shard); err != nil {
				return fmt.Errorf("put shard %q to peer %s: %w", shardName, peerID, err)
			}
		}
	}

	userIndexKey := deriveUserIndexKey(userID)
	headKey := deriveHeadKey(userID, datasetID)

	userIndexSuffix, err := randomCatalogSuffix()
	if err != nil {
		return fmt.Errorf("generate user index suffix: %w", err)
	}

	headSuffix, err := randomCatalogSuffix()
	if err != nil {
		return fmt.Errorf("generate head suffix: %w", err)
	}

	manifestSuffix, err := randomCatalogSuffix()
	if err != nil {
		return fmt.Errorf("generate manifest suffix: %w", err)
	}

	userIndexData := backup.SerializeUserIndex(userIndex)
	headData := backup.SerializeHead(head)
	wrappedDatasetData := backup.SerializeWrappedDataset(wrappedDataset)
	headCatalogData := append(headData, wrappedDatasetData...)
	manifestData := backup.SerializeManifestEnvelope(manifest)

	var catalogObjects []catalogObject

	for i := range 12 {
		userIndexName := fmt.Sprintf("userindex:%x:%s:%02d", userIndexKey, userIndexSuffix, i)
		headName := fmt.Sprintf("head:%x:%s:%02d", headKey, headSuffix, i)
		manifestName := fmt.Sprintf("manifest:%x:%s:%02d", manifest.ManifestID, manifestSuffix, i)

		catalogObjects = append(catalogObjects,
			catalogObject{
				name:       userIndexName,
				objectType: 1,
				data:       userIndexData,
			},
			catalogObject{
				name:       headName,
				objectType: 2,
				data:       headCatalogData,
			},
			catalogObject{
				name:       manifestName,
				objectType: 4,
				data:       manifestData,
			},
		)
	}

	catalogNames := make([]string, 0, len(catalogObjects))

	for _, object := range catalogObjects {
		catalogNames = append(catalogNames, object.name)
	}

	catalogPlacements := place.Place(livePeers, catalogNames)

	for peerID, objectNames := range catalogPlacements {
		for _, objectName := range objectNames {
			var object *catalogObject

			for i := range catalogObjects {
				if catalogObjects[i].name == objectName {
					object = &catalogObjects[i]
					break
				}
			}

			if object == nil {
				return fmt.Errorf("catalog object %q not found", objectName)
			}

			if err := network.PutCatalog(ctx, h, peerID, object.name, object.objectType, object.data, ed25519PublicKey); err != nil {
				return fmt.Errorf("put catalog %q to peer %s: %w", object.name, peerID, err)
			}
		}
	}

	return nil
}
