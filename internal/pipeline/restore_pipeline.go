package pipeline

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/0xh4ty/quailfs/internal/backup"
	"github.com/0xh4ty/quailfs/internal/codec"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/0xh4ty/quailfs/internal/network"
	"github.com/0xh4ty/quailfs/pkg/types"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
)

type FileToRestore struct {
	Path     string
	ChunkIDs [][]byte
}

func Restore(
	ctx context.Context,
	kad *dht.IpfsDHT,
	datasetID []byte,
	datasetKey []byte,
	files []FileToRestore,
	chunkIndex map[string]types.MChunk,
	stripeIndex map[string]types.MStripe,
	destDir string,
) ([]string, error) {
	if kad == nil {
		return nil, fmt.Errorf("DHT is not initialized")
	}

	if err := os.MkdirAll(destDir, 0700); err != nil {
		return nil, fmt.Errorf("create restore directory %q: %w", destDir, err)
	}

	h := kad.Host()

	stripeCache := make(map[string][]byte)

	var written []string

	for _, file := range files {
		if len(file.ChunkIDs) == 0 {
			log.Printf("Restore: %q has no chunks, skipping", file.Path)
			continue
		}

		var stripeOrder []string
		seenStripe := make(map[string]struct{})

		for _, chunkID := range file.ChunkIDs {
			chunkKey := hex.EncodeToString(chunkID)
			mchunk, ok := chunkIndex[chunkKey]
			if !ok {
				return nil, fmt.Errorf("restore %q: chunk %x not found in manifest index", file.Path, chunkID)
			}
			if len(mchunk.StripeID) == 0 {
				return nil, fmt.Errorf("restore %q: chunk %x missing stripe ID", file.Path, chunkID)
			}

			stripeKey := hex.EncodeToString(mchunk.StripeID)
			if _, dup := seenStripe[stripeKey]; dup {
				continue
			}
			seenStripe[stripeKey] = struct{}{}
			stripeOrder = append(stripeOrder, stripeKey)
		}

		var packedSerialized [][]byte

		for _, stripeKey := range stripeOrder {
			serialized, cached := stripeCache[stripeKey]
			if !cached {
				mstripe, ok := stripeIndex[stripeKey]
				if !ok {
					return nil, fmt.Errorf("restore %q: stripe %s not found in manifest index", file.Path, stripeKey)
				}

				var err error
				serialized, err = fetchAndDecryptStripe(ctx, h, kad, datasetID, datasetKey, mstripe)
				if err != nil {
					return nil, fmt.Errorf("restore %q: stripe %s: %w", file.Path, stripeKey, err)
				}

				stripeCache[stripeKey] = serialized
			}

			packedSerialized = append(packedSerialized, serialized)
		}

		packedPlain := backup.DeserializePackedPlainCollection(packedSerialized)
		compressedQChunks := backup.UnpackChunks(packedPlain)

		qchunks, err := codec.Decompressor(compressedQChunks)
		if err != nil {
			return nil, fmt.Errorf("restore %q: decompress: %w", file.Path, err)
		}

		data, err := backup.Dechunker(qchunks)
		if err != nil {
			return nil, fmt.Errorf("restore %q: reassemble: %w", file.Path, err)
		}

		outPath := filepath.Join(destDir, filepath.FromSlash(file.Path))

		if err := os.MkdirAll(filepath.Dir(outPath), 0700); err != nil {
			return nil, fmt.Errorf("restore %q: create directory: %w", file.Path, err)
		}

		if err := os.WriteFile(outPath, data, 0600); err != nil {
			return nil, fmt.Errorf("restore %q: write file: %w", file.Path, err)
		}

		log.Printf("Restore: wrote %q (%d bytes)", outPath, len(data))
		written = append(written, outPath)
	}

	return written, nil
}

func fetchAndDecryptStripe(
	ctx context.Context,
	h host.Host,
	kad *dht.IpfsDHT,
	datasetID []byte,
	datasetKey []byte,
	mstripe types.MStripe,
) ([]byte, error) {
	n := len(mstripe.ShardNames)
	if n == 0 {
		return nil, fmt.Errorf("stripe has no shard names")
	}

	shards := make([][]byte, n)
	have := 0

	for i, shardNameBytes := range mstripe.ShardNames {
		if uint64(have) >= mstripe.K {
			break
		}

		shardName := string(shardNameBytes)

		shardCID, err := network.ShardCID(shardName)
		if err != nil {
			log.Printf("Restore: shard %s: bad CID: %v", shardName, err)
			continue
		}

		findCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		providers, err := network.FindProviders(findCtx, kad, shardCID, 10)
		cancel()
		if err != nil || len(providers) == 0 {
			log.Printf("Restore: shard %s: no providers found: %v", shardName, err)
			continue
		}

		var data []byte
		for _, p := range providers {
			if p.ID == h.ID() {
				continue
			}

			getCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			d, err := network.GetShard(getCtx, h, p.ID, shardName)
			cancel()
			if err != nil {
				log.Printf("Restore: shard %s from %s: %v", shardName, p.ID, err)
				continue
			}
			data = d
			break
		}

		if data == nil {
			continue
		}

		shards[i] = data
		have++
	}

	if have < int(mstripe.K) {
		return nil, fmt.Errorf("only retrieved %d/%d required shard(s)", have, mstripe.K)
	}

	encryptedStripe, err := codec.DecodeStripe(shards)
	if err != nil {
		return nil, fmt.Errorf("decode stripe: %w", err)
	}

	stripeKeyBytes, err := keys.DeriveStripeKey(datasetKey, datasetID, mstripe.StripeID)
	if err != nil {
		return nil, fmt.Errorf("derive stripe key: %w", err)
	}

	nonce96, err := keys.DeriveNonce96(stripeKeyBytes, datasetID, mstripe.StripeID)
	if err != nil {
		return nil, fmt.Errorf("derive nonce: %w", err)
	}

	serialized, err := codec.DecryptStripe(stripeKeyBytes, nonce96, encryptedStripe)
	if err != nil {
		return nil, fmt.Errorf("decrypt stripe: %w", err)
	}

	return serialized, nil
}
