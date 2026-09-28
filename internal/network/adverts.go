package network

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/ipfs/go-cid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	mh "github.com/multiformats/go-multihash"
	"go.etcd.io/bbolt"
	"log"
	"strings"
	"time"
)

const AdInterval = 1 * time.Minute

func ShardCID(shardName string) (cid.Cid, error) {
	sum, err := mh.Sum([]byte(shardName), mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, sum), nil
}

func KeyCID(key []byte) (cid.Cid, error) {
	if len(key) != 32 {
		return cid.Undef, fmt.Errorf("lookup key must be 32 bytes, got %d", len(key))
	}
	sum, err := mh.Encode(key, mh.SHA2_256)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, sum), nil
}

func catalogKeyFromName(name string) ([]byte, bool) {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return nil, false
	}
	key, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	return key, true
}

func listKeys(db *bbolt.DB, bucket string) ([]string, error) {
	var keys []string
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, _ []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	})
	return keys, err
}

func advertise(ctx context.Context, kad *dht.IpfsDHT, db *bbolt.DB) {
	var cids []cid.Cid
	seen := make(map[string]struct{})

	add := func(c cid.Cid, err error) {
		if err != nil {
			return
		}
		if _, dup := seen[c.KeyString()]; dup {
			return
		}
		seen[c.KeyString()] = struct{}{}
		cids = append(cids, c)
	}

	shardNames, err := listKeys(db, "inventory")
	if err != nil {
		log.Printf("Provider ads: list shards: %v", err)
	}
	for _, name := range shardNames {
		add(ShardCID(name))
	}

	catalogNames, err := listKeys(db, "catalogs")
	if err != nil {
		log.Printf("Provider ads: list catalogs: %v", err)
	}
	for _, name := range catalogNames {
		if key, ok := catalogKeyFromName(name); ok {
			add(KeyCID(key))
		}
	}

	advertised := 0
	for _, c := range cids {
		if ctx.Err() != nil {
			break
		}
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := Provide(pctx, kad, c)
		cancel()
		if err != nil {
			log.Printf("Provider ads: provide %s failed: %v", c, err)
			continue
		}
		advertised++
	}

	log.Printf("Provider ads: advertised %d/%d key(s) (%d shard(s), %d catalog object(s) held)",
		advertised, len(cids), len(shardNames), len(catalogNames))
}

func StartProviderAds(ctx context.Context, kad *dht.IpfsDHT, db *bbolt.DB) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}

		ticker := time.NewTicker(AdInterval)
		defer ticker.Stop()

		for {
			roundCtx, cancel := context.WithTimeout(ctx, AdInterval)
			advertise(roundCtx, kad, db)
			cancel()

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
