package pipeline

import (
	"context"
	crand "crypto/rand"
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
	"log"
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

	chunkStripe := make(map[string][]byte)

	for _, qchunk := range qchunks {
		mChunks = append(mChunks, types.MChunk{
			ChunkID: qchunk.ChunkID,
			Size:    uint64(len(qchunk.Data)),
		})

		file.ChunkIDs = append(file.ChunkIDs, qchunk.ChunkID)
	}

	for i, stripe := range packedStripeSerialized {
		stripeID := keys.DeriveStripeID(datasetID, stripe)

		for _, qchunk := range packedStripePlain[i].QChunks {
			chunkStripe[string(qchunk.ChunkID)] = stripeID
		}

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

	for i := range mChunks {
		stripeID, ok := chunkStripe[string(mChunks[i].ChunkID)]
		if !ok {
			return types.File{}, nil, nil, backupShards{}, fmt.Errorf("chunk %x not found in any stripe", mChunks[i].ChunkID)
		}

		mChunks[i].StripeID = stripeID
	}

	return file, mChunks, mStripes, backupShards{
		stripeIDs: stripeIDs,
		shards:    shardCollection,
	}, nil
}

func randomCatalogSuffix() (string, error) {
	buf := make([]byte, 16)

	if _, err := crand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

func Backup(ctx context.Context, paths []string, userID []byte, datasetID []byte, datasetKey []byte, catalogKey []byte, label string, generation uint64, parentManifestID []byte, userX25519Pubkey []byte, ed25519PrivateKey []byte, ed25519PublicKey []byte, kad *dht.IpfsDHT) (BackupResult, error) {
	log.Printf("Backup started")
	log.Printf("Backup: %d path(s)", len(paths))

	var stripeIDs [][]byte
	var shardCollection [][][]byte

	var trees []types.Tree
	var mChunks []types.MChunk
	var mStripes []types.MStripe

	for _, rootPath := range paths {
		log.Printf("Backup: processing path %q", rootPath)

		info, err := os.Stat(rootPath)
		if err != nil {
			log.Printf("Backup: stat failed for %q: %v", rootPath, err)
			return BackupResult{}, fmt.Errorf("stat %q: %w", rootPath, err)
		}

		tree := types.Tree{
			RootDirectory: filepath.Base(rootPath),
		}

		if info.IsDir() {
			log.Printf("Backup: walking directory %q", rootPath)

			err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					log.Printf("Backup: walk error at %q: %v", path, err)
					return err
				}

				if info.IsDir() {
					return nil
				}

				log.Printf("Backup: processing file %q", path)

				file, chunks, stripes, shards, err := backupFile(path, datasetID, datasetKey)
				if err != nil {
					log.Printf("Backup: backupFile failed for %q: %v", path, err)
					return err
				}

				log.Printf("Backup: file %q produced %d chunk(s), %d stripe(s), %d shard set(s)", path, len(chunks), len(stripes), len(shards.shards))

				tree.Files = append(tree.Files, file)
				mChunks = append(mChunks, chunks...)
				mStripes = append(mStripes, stripes...)
				stripeIDs = append(stripeIDs, shards.stripeIDs...)
				shardCollection = append(shardCollection, shards.shards...)

				return nil
			})

			if err != nil {
				log.Printf("Backup: directory backup failed for %q: %v", rootPath, err)
				return BackupResult{}, fmt.Errorf("backup directory %q: %w", rootPath, err)
			}
		} else {
			log.Printf("Backup: processing file %q", rootPath)

			file, chunks, stripes, shards, err := backupFile(rootPath, datasetID, datasetKey)
			if err != nil {
				log.Printf("Backup: backupFile failed for %q: %v", rootPath, err)
				return BackupResult{}, err
			}

			log.Printf("Backup: file produced %d chunk(s), %d stripe(s), %d shard set(s)", len(chunks), len(stripes), len(shards.shards))

			tree.Files = append(tree.Files, file)
			mChunks = append(mChunks, chunks...)
			mStripes = append(mStripes, stripes...)
			stripeIDs = append(stripeIDs, shards.stripeIDs...)
			shardCollection = append(shardCollection, shards.shards...)
		}

		trees = append(trees, tree)
	}

	log.Printf("Backup: local processing complete: %d chunk(s), %d stripe(s), %d shard set(s)", len(mChunks), len(mStripes), len(shardCollection))

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
		log.Printf("Backup: create manifest failed: %v", err)
		return BackupResult{}, fmt.Errorf("create manifest: %w", err)
	}

	log.Printf("Backup: manifest created: %x", manifest.ManifestID)

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
		log.Printf("Backup: create wrapped dataset failed: %v", err)
		return BackupResult{}, fmt.Errorf("create wrapped dataset: %w", err)
	}

	head := catalog.CreateHead(
		1,
		userID,
		datasetID,
		generation,
		manifest.ManifestID,
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
		log.Printf("Backup: DHT is nil")
		return BackupResult{}, fmt.Errorf("DHT is not initialized")
	}

	h := kad.Host()

	log.Printf("Backup: local PeerID: %s", h.ID())

	livePeers := make([]peer.ID, 0)

	for _, p := range kad.RoutingTable().ListPeers() {
		if p == h.ID() {
			continue
		}

		log.Printf("Backup: discovered peer: %s", p)
		livePeers = append(livePeers, p)
	}

	log.Printf("Backup: discovered %d peer(s)", len(livePeers))

	if len(livePeers) == 0 {
		return BackupResult{}, fmt.Errorf("no peers discovered through DHT")
	}

	var objectNames []string

	for _, stripes := range mStripes {
		for _, shardName := range stripes.ShardNames {
			objectNames = append(objectNames, string(shardName))
		}
	}

	log.Printf("Backup: %d shard object(s) to place", len(objectNames))

	placements := place.Place(livePeers, objectNames)

	for peerID, shardNames := range placements {
		log.Printf("Backup: peer %s assigned %d shard(s)", peerID, len(shardNames))

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
				log.Printf("Backup: shard %q not found in shard collection", shardName)
				return BackupResult{}, fmt.Errorf("shard %q not found", shardName)
			}

			log.Printf("Backup: sending shard %s to peer %s (%d bytes)", shardName, peerID, len(shard))

			if err := network.PutShard(ctx, h, peerID, shardName, shard); err != nil {
				log.Printf("Backup: PUT_SHARD failed for %s -> %s: %v", shardName, peerID, err)
				return BackupResult{}, fmt.Errorf("put shard %q to peer %s: %w", shardName, peerID, err)
			}

			log.Printf("Backup: shard %s sent successfully to peer %s", shardName, peerID)
		}
	}

	log.Printf("Backup: all shards uploaded successfully")

	userIndexKey := keys.DeriveUserIndexKey(userID)
	headKey := keys.DeriveHeadKey(userID, datasetID)

	userIndexSuffix, err := randomCatalogSuffix()
	if err != nil {
		log.Printf("Backup: generate user index suffix failed: %v", err)
		return BackupResult{}, fmt.Errorf("generate user index suffix: %w", err)
	}

	headSuffix, err := randomCatalogSuffix()
	if err != nil {
		log.Printf("Backup: generate head suffix failed: %v", err)
		return BackupResult{}, fmt.Errorf("generate head suffix: %w", err)
	}

	manifestSuffix, err := randomCatalogSuffix()
	if err != nil {
		log.Printf("Backup: generate manifest suffix failed: %v", err)
		return BackupResult{}, fmt.Errorf("generate manifest suffix: %w", err)
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

	log.Printf("Backup: created %d catalog object replicas", len(catalogObjects))

	catalogNames := make([]string, 0, len(catalogObjects))

	for _, object := range catalogObjects {
		catalogNames = append(catalogNames, object.name)
	}

	catalogPlacements := place.Place(livePeers, catalogNames)

	for peerID, objectNames := range catalogPlacements {
		log.Printf("Backup: peer %s assigned %d catalog object(s)", peerID, len(objectNames))

		for _, objectName := range objectNames {
			var object *catalogObject

			for i := range catalogObjects {
				if catalogObjects[i].name == objectName {
					object = &catalogObjects[i]
					break
				}
			}

			if object == nil {
				log.Printf("Backup: catalog object %q not found", objectName)
				return BackupResult{}, fmt.Errorf("catalog object %q not found", objectName)
			}

			log.Printf("Backup: sending catalog %s to peer %s", object.name, peerID)

			if err := network.PutCatalog(ctx, h, peerID, object.name, object.objectType, object.data, ed25519PublicKey); err != nil {
				log.Printf("Backup: PUT_CATALOG failed for %s -> %s: %v", object.name, peerID, err)
				return BackupResult{}, fmt.Errorf("put catalog %q to peer %s: %w", object.name, peerID, err)
			}

			log.Printf("Backup: catalog %s sent successfully to peer %s", object.name, peerID)
		}
	}

	log.Printf("Backup completed successfully")

	return BackupResult{
		Manifest:        manifest,
		Head:            head,
		WrappedDataset:  wrappedDataset,
		UserIndex:       userIndex,
		StripeIDs:       stripeIDs,
		ShardCollection: shardCollection,
	}, nil
}
