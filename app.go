package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/0xh4ty/quailfs/internal/catalog"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/0xh4ty/quailfs/internal/network"
	"github.com/0xh4ty/quailfs/internal/pipeline"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type App struct {
	ctx context.Context

	mu sync.RWMutex

	unlocked bool

	userID            []byte
	ed25519PrivateKey []byte
	ed25519PublicKey  []byte
	x25519PrivateKey  []byte
	x25519PublicKey   []byte
	libp2pPrivateKey  crypto.PrivKey

	datasets map[string]DatasetSession

	bootstrapPeers []peer.AddrInfo
	libp2pHost     host.Host
	kad            *dht.IpfsDHT

	catalogObjects []network.CatalogBlob
}

type UserInfo struct {
	UserID    string
	PublicKey string
}

type DatasetSession struct {
	DatasetID  []byte
	DatasetKey []byte
	CatalogKey []byte
	DataKey    []byte
	NameKey    []byte
	Label      string
}

type DatasetInfo struct {
	ID         string
	Name       string
	Size       string
	Files      int
	LastBackup string
}

type FileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

func NewApp() *App {
	return &App{
		datasets: make(map[string]DatasetSession),
	}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ctx = ctx
}

func (a *App) IsUnlocked() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.unlocked
}

func (a *App) GenerateRecoveryPhrase() (string, error) {
	return keys.GenerateMnemonic(), nil
}

func (a *App) Unlock(recoveryPhrase string) (bool, error) {
	mnemonicSeed := keys.DeriveSeed(recoveryPhrase)

	ed25519Seed, err := keys.DeriveEd25519Seed(mnemonicSeed)
	if err != nil {
		return false, err
	}

	ed25519PrivateKey, ed25519PublicKey := keys.DeriveEd25519Keypair(ed25519Seed)

	libp2pPrivateKey, err := crypto.UnmarshalEd25519PrivateKey(ed25519PrivateKey)
	if err != nil {
		return false, err
	}

	x25519Seed, err := keys.DeriveX25519Seed(mnemonicSeed)
	if err != nil {
		return false, err
	}

	x25519PrivateKey, x25519PublicKey, err := keys.DeriveX25519Keypair(x25519Seed)
	if err != nil {
		return false, err
	}

	userID := keys.DeriveUserID(ed25519PublicKey)

	a.mu.Lock()
	defer a.mu.Unlock()

	a.userID = userID
	a.ed25519PrivateKey = ed25519PrivateKey
	a.ed25519PublicKey = ed25519PublicKey
	a.x25519PrivateKey = x25519PrivateKey
	a.x25519PublicKey = x25519PublicKey
	a.libp2pPrivateKey = libp2pPrivateKey
	a.unlocked = true

	return true, nil
}

func (a *App) GetUserInfo() (UserInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.unlocked {
		return UserInfo{}, errors.New("identity is locked")
	}

	return UserInfo{
		UserID:    hex.EncodeToString(a.userID),
		PublicKey: hex.EncodeToString(a.ed25519PublicKey),
	}, nil
}

func (a *App) ConfigureBootstrapNodes(addresses []string) error {
	if len(addresses) == 0 {
		return fmt.Errorf("no bootstrap nodes provided")
	}

	h, err := network.NewHost(
		a.libp2pPrivateKey,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create libp2p host: %w", err)
	}

	kad, err := dht.New(
		h,
		dht.Mode(dht.ModeServer),
	)
	if err != nil {
		h.Close()
		return fmt.Errorf("create DHT: %w", err)
	}

	var lastErr error
	connectedAny := false

	for _, address := range addresses {
		maddr, err := multiaddr.NewMultiaddr(address)
		if err != nil {
			lastErr = fmt.Errorf(
				"invalid bootstrap multiaddress %q: %w",
				address,
				err,
			)
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			lastErr = fmt.Errorf(
				"invalid bootstrap peer address %q: %w",
				address,
				err,
			)
			continue
		}

		connectCtx, cancel := context.WithTimeout(
			a.ctx,
			10*time.Second,
		)

		err = h.Connect(connectCtx, *addrInfo)

		cancel()

		if err != nil {
			lastErr = fmt.Errorf(
				"failed to connect to bootstrap node %q: %w",
				address,
				err,
			)
			continue
		}

		kad.RoutingTable().TryAddPeer(
			addrInfo.ID,
			true,
			false,
		)

		connectedAny = true
	}

	if !connectedAny {
		kad.Close()
		h.Close()

		if lastErr == nil {
			lastErr = fmt.Errorf("no valid bootstrap nodes")
		}

		return lastErr
	}

	if err := kad.Bootstrap(a.ctx); err != nil {
		kad.Close()
		h.Close()

		return fmt.Errorf("bootstrap DHT: %w", err)
	}

	a.libp2pHost = h
	a.kad = kad

	a.fetchCatalogObjects(a.ctx, kad)

	return nil
}

func (a *App) fetchCatalogObjects(ctx context.Context, kad *dht.IpfsDHT) {
	a.mu.RLock()
	userID := a.userID
	pub := a.ed25519PublicKey
	var datasetIDs [][]byte
	for _, d := range a.datasets {
		datasetIDs = append(datasetIDs, d.DatasetID)
	}
	a.mu.RUnlock()

	if len(userID) == 0 {
		return
	}

	lookupKeys := [][]byte{keys.DeriveUserIndexKey(userID)}
	for _, id := range datasetIDs {
		lookupKeys = append(lookupKeys, keys.DeriveHeadKey(userID, id))
	}

	var fetched []network.CatalogBlob

	for _, key := range lookupKeys {
		blobs, err := network.FetchCatalogObjects(ctx, kad, key)
		if err != nil {
			log.Printf("catalog: fetch failed: %v", err)
			continue
		}

		for _, b := range blobs {
			// Verify against our own key, never one supplied by the node.
			if err := catalog.VerifyCatalogObject(b.Type, b.Data, pub); err != nil {
				log.Printf("catalog: dropping %s: %v", b.Name, err)
				continue
			}
			fetched = append(fetched, b)
		}
	}

	a.mu.Lock()
	a.catalogObjects = fetched
	a.mu.Unlock()

	log.Printf("catalog: fetched %d verified object(s)", len(fetched))
}

func (a *App) CreateDataset(label string) (DatasetInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.unlocked {
		return DatasetInfo{}, errors.New("user is not unlocked")
	}

	label = strings.TrimSpace(label)
	if label == "" {
		return DatasetInfo{}, errors.New("dataset label cannot be empty")
	}

	datasetKey, err := keys.GenerateDatasetKey()
	if err != nil {
		return DatasetInfo{}, err
	}

	datasetID := keys.DeriveDatasetID(a.userID, label)

	catalogKey, err := keys.DeriveCatalogKey(datasetKey, datasetID)
	if err != nil {
		return DatasetInfo{}, err
	}

	dataKey, err := keys.DeriveDataKey(datasetKey, datasetID)
	if err != nil {
		return DatasetInfo{}, err
	}

	nameKey, err := keys.DeriveNameKey(datasetKey, datasetID)
	if err != nil {
		return DatasetInfo{}, err
	}

	a.datasets[hex.EncodeToString(datasetID)] = DatasetSession{
		DatasetID:  datasetID,
		DatasetKey: datasetKey,
		CatalogKey: catalogKey,
		DataKey:    dataKey,
		NameKey:    nameKey,
		Label:      label,
	}

	return DatasetInfo{
		ID:         hex.EncodeToString(datasetID),
		Name:       label,
		Size:       "0 B",
		Files:      0,
		LastBackup: "Never",
	}, nil
}

func (a *App) ListDirectory(path string) ([]FileEntry, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.unlocked {
		return nil, errors.New("identity is locked")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	files := make([]FileEntry, 0, len(entries))

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		fileType := "file"
		if entry.IsDir() {
			fileType = "directory"
		}

		files = append(files, FileEntry{
			Name: entry.Name(),
			Path: filepath.Join(path, entry.Name()),
			Type: fileType,
			Size: info.Size(),
		})
	}

	return files, nil
}

func (a *App) GetHomeDirectory() (string, error) {
	return os.UserHomeDir()
}

func (a *App) Backup(datasetID string, paths []string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.unlocked {
		return errors.New("user is not unlocked")
	}

	dataset, ok := a.datasets[datasetID]
	if !ok {
		return errors.New("dataset not found")
	}

	return pipeline.Backup(
		a.ctx,
		paths,
		a.userID,
		dataset.DatasetID,
		dataset.DatasetKey,
		dataset.CatalogKey,
		dataset.Label,
		1,
		nil,
		a.x25519PublicKey,
		a.ed25519PrivateKey,
		a.ed25519PublicKey,
		a.kad,
	)
}
