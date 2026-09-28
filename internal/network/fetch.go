package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"log"
	"time"
)

func FetchCatalogObjects(ctx context.Context, kad *dht.IpfsDHT, key []byte) ([]CatalogBlob, error) {
	c, err := KeyCID(key)
	if err != nil {
		return nil, err
	}

	findCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	providers, err := FindProviders(findCtx, kad, c, 20)
	cancel()
	if err != nil {
		return nil, err
	}

	h := kad.Host()
	keyHex := hex.EncodeToString(key)
	seen := make(map[[32]byte]struct{})
	var out []CatalogBlob

	log.Printf("Catalog fetch: %d provider(s) for key %s", len(providers), keyHex[:12])

	for _, p := range providers {
		if p.ID == h.ID() {
			continue
		}

		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		blobs, err := GetCatalog(rctx, h, p.ID, keyHex)
		cancel()
		if err != nil {
			log.Printf("Catalog fetch: peer %s: %v", p.ID, err)
			continue
		}

		for _, b := range blobs {
			sum := sha256.Sum256(b.Data)
			if _, dup := seen[sum]; dup {
				continue
			}
			seen[sum] = struct{}{}
			out = append(out, b)
		}
	}

	return out, nil
}
