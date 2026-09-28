package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/0xh4ty/quailfs/internal/backup"
	"github.com/0xh4ty/quailfs/internal/catalog"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/0xh4ty/quailfs/internal/network"
	"github.com/0xh4ty/quailfs/internal/pipeline"
	"github.com/0xh4ty/quailfs/pkg/types"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	net "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	Size       string `json:"size"`
	Files      int    `json:"files"`
	LastBackup string `json:"lastBackup"`
	Generation uint64 `json:"generation"`
}

type NodeInfo struct {
	PeerID    string `json:"peerId"`
	Status    string `json:"status"`
	Latency   string `json:"latency"`
	Bootstrap bool   `json:"bootstrap"`
}

type FileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

type restoreFileEntry struct {
	ChunkIDs [][]byte
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

	a.mu.Lock()
	a.libp2pHost = h
	a.kad = kad
	a.bootstrapPeers = nil
	for _, address := range addresses {
		maddr, err := multiaddr.NewMultiaddr(address)
		if err != nil {
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			continue
		}

		a.bootstrapPeers = append(a.bootstrapPeers, *addrInfo)
	}
	a.mu.Unlock()

	a.fetchCatalogObjects(a.ctx, kad)

	return nil
}

func (a *App) fetchCatalogObjects(ctx context.Context, kad *dht.IpfsDHT) {
	a.mu.RLock()
	userID := append([]byte(nil), a.userID...)
	pub := append([]byte(nil), a.ed25519PublicKey...)
	x25519PrivateKey := append([]byte(nil), a.x25519PrivateKey...)
	a.mu.RUnlock()

	if len(userID) == 0 {
		return
	}

	var fetched []network.CatalogBlob

	// -------------------Fetch UserIndex objects------------------

	userIndexKey := keys.DeriveUserIndexKey(userID)

	userIndexBlobs, err := network.FetchCatalogObjects(ctx, kad, userIndexKey)
	if err != nil {
		log.Printf("catalog: user index fetch failed: %v", err)
		return
	}

	var datasetIDs [][]byte

	for _, blob := range userIndexBlobs {
		if err := catalog.VerifyCatalogObject(blob.Type, blob.Data, pub); err != nil {
			log.Printf("catalog: dropping user index %s: %v", blob.Name, err)
			continue
		}

		fetched = append(fetched, blob)

		userIndex, err := backup.DeserializeUserIndex(blob.Data)
		if err != nil {
			log.Printf(
				"catalog: failed to deserialize user index %s: %v",
				blob.Name,
				err,
			)
			continue
		}

		for _, dataset := range userIndex.Body.Datasets {
			datasetID := append([]byte(nil), dataset.DatasetID...)

			alreadyExists := false

			for _, existingID := range datasetIDs {
				if string(existingID) == string(datasetID) {
					alreadyExists = true
					break
				}
			}

			if !alreadyExists {
				datasetIDs = append(datasetIDs, datasetID)
			}
		}
	}

	// ---------Fetch Head objects and unwrap DatasetKeys----------

	var manifestIDs [][]byte
	manifestCatalogKeys := make(map[string][]byte)

	log.Printf("catalog: starting head fetch for %d dataset(s)", len(datasetIDs))

	for _, datasetID := range datasetIDs {
		log.Printf("catalog: processing dataset %x", datasetID)

		headKey := keys.DeriveHeadKey(userID, datasetID)

		log.Printf(
			"catalog: derived head key %x for dataset %x",
			headKey,
			datasetID,
		)

		headBlobs, err := network.FetchCatalogObjects(ctx, kad, headKey)
		if err != nil {
			log.Printf(
				"catalog: head fetch failed for dataset %x: %v",
				datasetID,
				err,
			)
			continue
		}

		log.Printf(
			"catalog: fetched %d head object(s) for dataset %x",
			len(headBlobs),
			datasetID,
		)

		for _, blob := range headBlobs {
			log.Printf(
				"catalog: processing head blob %s, type=%d, size=%d",
				blob.Name,
				blob.Type,
				len(blob.Data),
			)

			log.Printf("catalog: verifying head %s", blob.Name)

			if err := catalog.VerifyCatalogObject(blob.Type, blob.Data, pub); err != nil {
				log.Printf("catalog: dropping head %s: %v", blob.Name, err)
				continue
			}

			log.Printf("catalog: head %s verified", blob.Name)

			log.Printf("catalog: deserializing head %s", blob.Name)

			head, wrappedDataset, err := backup.DeserializeHeadCatalog(blob.Data)
			if err != nil {
				log.Printf(
					"catalog: failed to deserialize head %s: %v",
					blob.Name,
					err,
				)
				continue
			}

			log.Printf(
				"catalog: head %s deserialized, manifest ID=%x",
				blob.Name,
				head.Body.ManifestID,
			)

			log.Printf("catalog: unwrapping dataset key for head %s", blob.Name)

			datasetKey, err := keys.UnwrapDatasetKey(
				wrappedDataset,
				x25519PrivateKey,
			)
			if err != nil {
				log.Printf(
					"catalog: failed to unwrap dataset key %s: %v",
					blob.Name,
					err,
				)
				continue
			}

			log.Printf(
				"catalog: dataset key unwrapped for head %s, key length=%d",
				blob.Name,
				len(datasetKey),
			)

			log.Printf(
				"catalog: deriving catalog key for dataset %x",
				datasetID,
			)

			catalogKey, err := keys.DeriveCatalogKey(
				datasetKey,
				datasetID,
			)
			if err != nil {
				log.Printf(
					"catalog: failed to derive catalog key for dataset %x: %v",
					datasetID,
					err,
				)
				continue
			}

			log.Printf(
				"catalog: catalog key derived for dataset %x, key length=%d",
				datasetID,
				len(catalogKey),
			)

			log.Printf(
				"catalog: deriving data key for dataset %x",
				datasetID,
			)

			dataKey, err := keys.DeriveDataKey(
				datasetKey,
				datasetID,
			)
			if err != nil {
				log.Printf(
					"catalog: failed to derive data key for dataset %x: %v",
					datasetID,
					err,
				)
				continue
			}

			log.Printf(
				"catalog: data key derived for dataset %x, key length=%d",
				datasetID,
				len(dataKey),
			)

			log.Printf(
				"catalog: deriving name key for dataset %x",
				datasetID,
			)

			nameKey, err := keys.DeriveNameKey(
				datasetKey,
				datasetID,
			)
			if err != nil {
				log.Printf(
					"catalog: failed to derive name key for dataset %x: %v",
					datasetID,
					err,
				)
				continue
			}

			log.Printf(
				"catalog: name key derived for dataset %x, key length=%d",
				datasetID,
				len(nameKey),
			)

			log.Printf(
				"catalog: storing dataset session for dataset %x",
				datasetID,
			)

			a.mu.Lock()
			a.datasets[hex.EncodeToString(datasetID)] = DatasetSession{
				DatasetID:  append([]byte(nil), datasetID...),
				DatasetKey: append([]byte(nil), datasetKey...),
				CatalogKey: append([]byte(nil), catalogKey...),
				DataKey:    append([]byte(nil), dataKey...),
				NameKey:    append([]byte(nil), nameKey...),
				Label:      wrappedDataset.Body.Label,
			}
			a.mu.Unlock()

			log.Printf(
				"catalog: dataset session stored for dataset %x, label=%q",
				datasetID,
				wrappedDataset.Body.Label,
			)

			fetched = append(fetched, blob)

			log.Printf(
				"catalog: extracting manifest ID from head %s",
				blob.Name,
			)

			manifestID := append([]byte(nil), head.Body.ManifestID...)

			log.Printf(
				"catalog: manifest ID=%x",
				manifestID,
			)

			log.Printf(
				"catalog: storing catalog key for manifest %x",
				manifestID,
			)

			manifestCatalogKeys[hex.EncodeToString(manifestID)] = append(
				[]byte(nil),
				catalogKey...,
			)

			log.Printf(
				"catalog: manifest catalog key stored for %x",
				manifestID,
			)

			alreadyExists := false

			for _, existingID := range manifestIDs {
				if string(existingID) == string(manifestID) {
					alreadyExists = true
					break
				}
			}

			if alreadyExists {
				log.Printf(
					"catalog: manifest %x already exists, skipping duplicate",
					manifestID,
				)
				continue
			}

			manifestIDs = append(manifestIDs, manifestID)

			log.Printf(
				"catalog: added manifest %x, total manifests=%d",
				manifestID,
				len(manifestIDs),
			)
		}
	}

	log.Printf(
		"catalog: finished head processing, dataset count=%d, manifest count=%d",
		len(datasetIDs),
		len(manifestIDs),
	)

	// -----------------Fetch Manifest objects---------------------

	for _, manifestID := range manifestIDs {
		log.Printf("catalog: fetching manifest objects for %x", manifestID)

		manifestBlobs, err := network.FetchCatalogObjects(ctx, kad, manifestID)
		if err != nil {
			log.Printf(
				"catalog: manifest fetch failed for %x: %v",
				manifestID,
				err,
			)
			continue
		}

		log.Printf(
			"catalog: fetched %d manifest object(s) for %x",
			len(manifestBlobs),
			manifestID,
		)

		catalogKey, ok := manifestCatalogKeys[hex.EncodeToString(manifestID)]
		if !ok {
			log.Printf(
				"catalog: catalog key not found for manifest %x",
				manifestID,
			)
			continue
		}

		for _, blob := range manifestBlobs {
			log.Printf(
				"catalog: processing manifest %s, type=%d, size=%d",
				blob.Name,
				blob.Type,
				len(blob.Data),
			)

			if err := catalog.VerifyCatalogObject(blob.Type, blob.Data, pub); err != nil {
				log.Printf("catalog: dropping manifest %s: %v", blob.Name, err)
				continue
			}

			log.Printf("catalog: manifest %s verified", blob.Name)

			manifest, err := backup.DeserializeManifestEnvelope(blob.Data)
			if err != nil {
				log.Printf(
					"catalog: failed to deserialize manifest %s: %v",
					blob.Name,
					err,
				)
				continue
			}

			log.Printf("catalog: manifest %s deserialized", blob.Name)

			if err := catalog.OpenManifestEnvelope(&manifest, catalogKey); err != nil {
				log.Printf(
					"catalog: failed to open manifest %s: %v",
					blob.Name,
					err,
				)
				continue
			}

			log.Printf("catalog: manifest %s opened", blob.Name)

			fetched = append(fetched, blob)
		}

		log.Printf("catalog: finished processing manifest %x", manifestID)
	}

	a.mu.Lock()
	a.catalogObjects = fetched
	a.mu.Unlock()

	log.Printf(
		"catalog: fetched %d verified object(s), %d dataset(s), %d manifest(s)",
		len(fetched),
		len(datasetIDs),
		len(manifestIDs),
	)
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

func (a *App) ListDatasets() ([]DatasetInfo, error) {
	a.mu.RLock()
	if !a.unlocked {
		a.mu.RUnlock()
		return nil, errors.New("identity is locked")
	}
	datasets := make([]DatasetSession, 0, len(a.datasets))
	for _, dataset := range a.datasets {
		datasets = append(datasets, dataset)
	}
	a.mu.RUnlock()

	result := make([]DatasetInfo, 0, len(datasets))
	for _, dataset := range datasets {
		files, generation, createdAt, err := a.getDatasetFiles(dataset)
		if err != nil {
			return nil, fmt.Errorf("dataset %x: %w", dataset.DatasetID, err)
		}

		var totalSize int64
		for _, file := range files {
			if file.Type == "file" {
				totalSize += file.Size
			}
		}

		lastBackup := formatLastBackup(createdAt)

		result = append(result, DatasetInfo{
			ID:         hex.EncodeToString(dataset.DatasetID),
			Name:       dataset.Label,
			Size:       formatBytes(totalSize),
			Files:      len(files),
			LastBackup: lastBackup,
			Generation: generation,
		})
	}

	return result, nil
}

func formatLastBackup(createdAt string) string {
	if createdAt == "" {
		return "Never"
	}

	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return createdAt
	}

	return t.Local().Format("Jan 2, 3:04 PM")
}

func (a *App) GetDatasetFiles(datasetID string) ([]FileEntry, error) {
	a.mu.RLock()
	if !a.unlocked {
		a.mu.RUnlock()
		return nil, errors.New("identity is locked")
	}
	dataset, ok := a.datasets[datasetID]
	kad := a.kad
	ctx := a.ctx
	a.mu.RUnlock()

	if !ok {
		return nil, errors.New("dataset not found")
	}
	if kad == nil {
		return nil, errors.New("DHT is not initialized")
	}

	files, _, _, err := a.getDatasetFilesWithNetwork(ctx, kad, dataset)
	return files, err
}

func (a *App) getDatasetFiles(dataset DatasetSession) ([]FileEntry, uint64, string, error) {
	a.mu.RLock()
	kad := a.kad
	ctx := a.ctx
	a.mu.RUnlock()

	if kad == nil {
		return nil, 0, "", errors.New("DHT is not initialized")
	}

	return a.getDatasetFilesWithNetwork(ctx, kad, dataset)
}

func (a *App) getDatasetFilesWithNetwork(ctx context.Context, kad *dht.IpfsDHT, dataset DatasetSession) ([]FileEntry, uint64, string, error) {
	a.mu.RLock()
	userID := append([]byte(nil), a.userID...)
	pub := append([]byte(nil), a.ed25519PublicKey...)
	a.mu.RUnlock()

	headKey := keys.DeriveHeadKey(userID, dataset.DatasetID)
	headBlobs, err := network.FetchCatalogObjects(ctx, kad, headKey)
	if err != nil {
		return nil, 0, "", fmt.Errorf("fetch dataset head: %w", err)
	}

	type headRef struct {
		manifestID []byte
		generation uint64
		createdAt  string
	}

	seenManifest := make(map[string]struct{})
	var heads []headRef

	for _, blob := range headBlobs {
		if blob.Type != 2 {
			continue
		}
		if err := catalog.VerifyCatalogObject(blob.Type, blob.Data, pub); err != nil {
			continue
		}

		head, wrapped, err := backup.DeserializeHeadCatalog(blob.Data)
		if err != nil {
			continue
		}
		if !bytes.Equal(head.Body.UserID, userID) || !bytes.Equal(head.Body.DatasetID, dataset.DatasetID) {
			continue
		}
		if !bytes.Equal(wrapped.Body.DatasetID, dataset.DatasetID) {
			continue
		}

		key := hex.EncodeToString(head.Body.ManifestID)
		if _, dup := seenManifest[key]; dup {
			continue
		}
		seenManifest[key] = struct{}{}

		heads = append(heads, headRef{
			manifestID: append([]byte(nil), head.Body.ManifestID...),
			generation: head.Body.Generation,
			createdAt:  head.Body.CreatedAt,
		})
	}

	if len(heads) == 0 {
		return nil, 0, "", errors.New("no valid head found for dataset")
	}

	entries := make(map[string]FileEntry)
	var maxGeneration uint64
	var latestCreatedAt string
	resolvedAny := false

	for _, h := range heads {
		chain, err := a.resolveManifestChain(ctx, kad, dataset, pub, h.manifestID)
		if err != nil {
			log.Printf("catalog: dataset %x: skipping head (manifest %x): %v", dataset.DatasetID, h.manifestID, err)
			continue
		}
		resolvedAny = true

		for i := len(chain) - 1; i >= 0; i-- {
			manifest := chain[i]
			chunkSizes := make(map[string]int64, len(manifest.ManifestPlain.MChunks))
			for _, chunk := range manifest.ManifestPlain.MChunks {
				chunkSizes[hex.EncodeToString(chunk.ChunkID)] = int64(chunk.Size)
			}

			for _, tree := range manifest.ManifestPlain.Trees {
				applyTreeFiles(entries, tree, tree.RootDirectory, chunkSizes)
			}

			for _, tombstone := range manifest.ManifestPlain.Tombstones {
				delete(entries, normalizeDatasetPath(tombstone))
			}
		}

		if h.generation > maxGeneration {
			maxGeneration = h.generation
			latestCreatedAt = h.createdAt
		}
	}

	if !resolvedAny {
		return nil, 0, "", errors.New("no head's manifest chain could be resolved")
	}

	result := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result, maxGeneration, latestCreatedAt, nil
}

func (a *App) resolveManifestChain(
	ctx context.Context,
	kad *dht.IpfsDHT,
	dataset DatasetSession,
	pub []byte,
	manifestID []byte,
) ([]types.ManifestEnvelope, error) {
	var chain []types.ManifestEnvelope
	seen := make(map[string]struct{})
	currentID := append([]byte(nil), manifestID...)

	for len(currentID) > 0 && !isZeroBytes(currentID) {
		key := hex.EncodeToString(currentID)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("manifest parent cycle detected at %x", currentID)
		}
		seen[key] = struct{}{}

		blobs, err := network.FetchCatalogObjects(ctx, kad, currentID)
		if err != nil {
			return nil, fmt.Errorf("fetch manifest %x: %w", currentID, err)
		}

		found := false
		for _, blob := range blobs {
			if blob.Type != 4 {
				continue
			}
			if err := catalog.VerifyCatalogObject(blob.Type, blob.Data, pub); err != nil {
				continue
			}

			manifest, err := backup.DeserializeManifestEnvelope(blob.Data)
			if err != nil {
				continue
			}
			if !bytes.Equal(manifest.ManifestID, currentID) {
				continue
			}
			if err := catalog.OpenManifestEnvelope(&manifest, dataset.CatalogKey); err != nil {
				continue
			}
			if !bytes.Equal(manifest.ManifestPlain.DatasetID, dataset.DatasetID) {
				continue
			}

			chain = append(chain, manifest)
			currentID = append([]byte(nil), manifest.ManifestPlain.ParentManifestID...)
			found = true
			break
		}

		if !found {
			return nil, fmt.Errorf("manifest %x could not be opened", currentID)
		}
	}

	return chain, nil
}

func applyTreeFiles(entries map[string]FileEntry, tree types.Tree, prefix string, chunkSizes map[string]int64) {
	prefix = normalizeDatasetPath(prefix)

	for _, file := range tree.Files {
		path := normalizeDatasetPath(filepath.Join(prefix, file.FileName))
		var size int64
		for _, chunkID := range file.ChunkIDs {
			size += chunkSizes[hex.EncodeToString(chunkID)]
		}
		entries[path] = FileEntry{
			Name: file.FileName,
			Path: path,
			Type: "file",
			Size: size,
		}
	}

	for _, child := range tree.ChildDirectories {
		if child != nil {
			applyTreeFiles(entries, *child, filepath.Join(prefix, child.RootDirectory), chunkSizes)
		}
	}
}

func normalizeDatasetPath(path string) string {
	return strings.Trim(filepath.ToSlash(path), "/")
}

func isZeroBytes(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", size)
	}
	if value >= 10 {
		return fmt.Sprintf("%.0f %s", value, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func (a *App) ListNodes() ([]NodeInfo, error) {
	a.mu.RLock()
	if !a.unlocked {
		a.mu.RUnlock()
		return nil, errors.New("identity is locked")
	}

	h := a.libp2pHost
	kad := a.kad
	bootstrapPeers := append([]peer.AddrInfo(nil), a.bootstrapPeers...)
	a.mu.RUnlock()

	if h == nil || kad == nil {
		return nil, errors.New("network is not initialized")
	}

	bootstrap := make(map[peer.ID]struct{}, len(bootstrapPeers))
	for _, addrInfo := range bootstrapPeers {
		bootstrap[addrInfo.ID] = struct{}{}
	}

	peerIDs := make(map[peer.ID]struct{})
	for _, peerID := range kad.RoutingTable().ListPeers() {
		if peerID != h.ID() {
			peerIDs[peerID] = struct{}{}
		}
	}
	for _, peerID := range h.Network().Peers() {
		if peerID != h.ID() {
			peerIDs[peerID] = struct{}{}
		}
	}

	result := make([]NodeInfo, 0, len(peerIDs))
	for peerID := range peerIDs {
		status := "Offline"
		if h.Network().Connectedness(peerID) == net.Connected {
			status = "Online"
		}

		_, isBootstrap := bootstrap[peerID]
		result = append(result, NodeInfo{
			PeerID:    peerID.String(),
			Status:    status,
			Latency:   "-",
			Bootstrap: isBootstrap,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PeerID < result[j].PeerID
	})

	return result, nil
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

	var generation uint64 = 1
	var parentManifestID []byte

	for _, blob := range a.catalogObjects {
		if blob.Type != 4 {
			continue
		}

		manifest, err := backup.DeserializeManifestEnvelope(blob.Data)
		if err != nil {
			continue
		}

		if err := catalog.OpenManifestEnvelope(&manifest, dataset.CatalogKey); err != nil {
			continue
		}

		if !bytes.Equal(manifest.ManifestPlain.DatasetID, dataset.DatasetID) {
			continue
		}

		if manifest.ManifestPlain.Generation >= generation {
			generation = manifest.ManifestPlain.Generation + 1
			parentManifestID = append([]byte(nil), manifest.ManifestID...)
		}
	}

	if parentManifestID == nil {
		parentManifestID = make([]byte, 32)
	}

	return pipeline.Backup(
		a.ctx,
		paths,
		a.userID,
		dataset.DatasetID,
		dataset.DatasetKey,
		dataset.CatalogKey,
		dataset.Label,
		generation,
		parentManifestID,
		a.x25519PublicKey,
		a.ed25519PrivateKey,
		a.ed25519PublicKey,
		a.kad,
	)
}

func (a *App) Restore(datasetID string, paths []string, destination string) (string, error) {
	a.mu.RLock()
	if !a.unlocked {
		a.mu.RUnlock()
		return "", errors.New("identity is locked")
	}
	dataset, ok := a.datasets[datasetID]
	kad := a.kad
	ctx := a.ctx
	userID := append([]byte(nil), a.userID...)
	pub := append([]byte(nil), a.ed25519PublicKey...)
	a.mu.RUnlock()

	if !ok {
		return "", errors.New("dataset not found")
	}
	if kad == nil {
		return "", errors.New("DHT is not initialized")
	}
	if len(paths) == 0 {
		return "", errors.New("no paths selected for restore")
	}

	fileIndex, chunkIndex, stripeIndex, err := a.resolveDatasetForRestore(ctx, kad, userID, pub, dataset)
	if err != nil {
		return "", fmt.Errorf("resolve dataset: %w", err)
	}

	var toRestore []pipeline.FileToRestore
	for _, p := range paths {
		normalized := normalizeDatasetPath(p)
		entry, ok := fileIndex[normalized]
		if !ok {
			return "", fmt.Errorf("path %q not found in dataset", p)
		}
		toRestore = append(toRestore, pipeline.FileToRestore{
			Path:     normalized,
			ChunkIDs: entry.ChunkIDs,
		})
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	destDir := filepath.Join(homeDir, ".node-data", "restored")

	written, err := pipeline.Restore(ctx, kad, dataset.DatasetID, dataset.DatasetKey, toRestore, chunkIndex, stripeIndex, destDir)
	if err != nil {
		return "", err
	}

	log.Printf("Restore: wrote %d file(s) to %s", len(written), destDir)

	return destDir, nil
}

func (a *App) resolveDatasetForRestore(
	ctx context.Context,
	kad *dht.IpfsDHT,
	userID []byte,
	pub []byte,
	dataset DatasetSession,
) (map[string]restoreFileEntry, map[string]types.MChunk, map[string]types.MStripe, error) {
	headKey := keys.DeriveHeadKey(userID, dataset.DatasetID)
	headBlobs, err := network.FetchCatalogObjects(ctx, kad, headKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetch dataset head: %w", err)
	}

	seenManifest := make(map[string]struct{})
	var manifestIDs [][]byte

	for _, blob := range headBlobs {
		if blob.Type != 2 {
			continue
		}
		if err := catalog.VerifyCatalogObject(blob.Type, blob.Data, pub); err != nil {
			continue
		}

		head, wrapped, err := backup.DeserializeHeadCatalog(blob.Data)
		if err != nil {
			continue
		}
		if !bytes.Equal(head.Body.UserID, userID) || !bytes.Equal(head.Body.DatasetID, dataset.DatasetID) {
			continue
		}
		if !bytes.Equal(wrapped.Body.DatasetID, dataset.DatasetID) {
			continue
		}

		key := hex.EncodeToString(head.Body.ManifestID)
		if _, dup := seenManifest[key]; dup {
			continue
		}
		seenManifest[key] = struct{}{}
		manifestIDs = append(manifestIDs, append([]byte(nil), head.Body.ManifestID...))
	}

	if len(manifestIDs) == 0 {
		return nil, nil, nil, errors.New("no valid head found for dataset")
	}

	fileIndex := make(map[string]restoreFileEntry)
	chunkIndex := make(map[string]types.MChunk)
	stripeIndex := make(map[string]types.MStripe)
	resolvedAny := false

	for _, manifestID := range manifestIDs {
		chain, err := a.resolveManifestChain(ctx, kad, dataset, pub, manifestID)
		if err != nil {
			log.Printf("restore: dataset %x: skipping manifest %x: %v", dataset.DatasetID, manifestID, err)
			continue
		}
		resolvedAny = true

		for i := len(chain) - 1; i >= 0; i-- {
			manifest := chain[i]

			for _, chunk := range manifest.ManifestPlain.MChunks {
				chunkIndex[hex.EncodeToString(chunk.ChunkID)] = chunk
			}
			for _, stripe := range manifest.ManifestPlain.MStripes {
				stripeIndex[hex.EncodeToString(stripe.StripeID)] = stripe
			}

			for _, tree := range manifest.ManifestPlain.Trees {
				applyTreeFilesForRestore(fileIndex, tree, tree.RootDirectory)
			}

			for _, tombstone := range manifest.ManifestPlain.Tombstones {
				delete(fileIndex, normalizeDatasetPath(tombstone))
			}
		}
	}

	if !resolvedAny {
		return nil, nil, nil, errors.New("no head's manifest chain could be resolved")
	}

	return fileIndex, chunkIndex, stripeIndex, nil
}

func applyTreeFilesForRestore(entries map[string]restoreFileEntry, tree types.Tree, prefix string) {
	prefix = normalizeDatasetPath(prefix)

	for _, file := range tree.Files {
		path := normalizeDatasetPath(filepath.Join(prefix, file.FileName))
		entries[path] = restoreFileEntry{
			ChunkIDs: file.ChunkIDs,
		}
	}

	for _, child := range tree.ChildDirectories {
		if child != nil {
			applyTreeFilesForRestore(entries, *child, filepath.Join(prefix, child.RootDirectory))
		}
	}
}
