package network

import (
	"context"
	"fmt"
	"github.com/ipfs/go-cid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
)

func Provide(ctx context.Context, kad *dht.IpfsDHT, key cid.Cid) error {
	if !key.Defined() {
		return fmt.Errorf("provider key is undefined")
	}

	if err := kad.Provide(ctx, key, true); err != nil {
		return fmt.Errorf("provide key: %w", err)
	}

	return nil
}

func FindProviders(ctx context.Context, kad *dht.IpfsDHT, key cid.Cid, limit int) ([]peer.AddrInfo, error) {
	if !key.Defined() {
		return nil, fmt.Errorf("provider key is undefined")
	}

	if limit <= 0 {
		return nil, fmt.Errorf("provider limit must be greater than zero")
	}

	providers := kad.FindProvidersAsync(ctx, key, limit)

	result := make([]peer.AddrInfo, 0, limit)

	for provider := range providers {
		result = append(result, provider)
	}

	return result, nil
}
