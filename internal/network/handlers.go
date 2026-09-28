package network

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	cat "github.com/0xh4ty/quailfs/internal/catalog"
	"github.com/0xh4ty/quailfs/internal/storage"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"go.etcd.io/bbolt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func RegisterHandlers(h host.Host, db *bbolt.DB) {
	h.SetStreamHandler(PutShardProtocol, func(stream network.Stream) {
		handlePutShard(stream, db)
	})

	h.SetStreamHandler(PutCatalogProtocol, func(stream network.Stream) {
		handlePutCatalog(stream, db)
	})

	h.SetStreamHandler(GetCatalogProtocol, func(stream network.Stream) {
		handleGetCatalog(stream, db)
	})

	h.SetStreamHandler(GetShardProtocol, func(stream network.Stream) {
		handleGetShard(stream, db)
	})
}

func handlePutShard(stream network.Stream, db *bbolt.DB) {
	defer stream.Close()

	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	shardName, err := readString(reader)
	if err != nil {
		writeResponse(writer, false)
		return
	}

	var shardSize uint64

	if err := binary.Read(reader, binary.BigEndian, &shardSize); err != nil {
		writeResponse(writer, false)
		return
	}

	if shardSize > uint64(^uint(0)>>1) {
		writeResponse(writer, false)
		return
	}

	shard := make([]byte, int(shardSize))

	if _, err := io.ReadFull(reader, shard); err != nil {
		writeResponse(writer, false)
		return
	}

	if !validShardName(shardName) {
		writeResponse(writer, false)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		writeResponse(writer, false)
		return
	}

	shardsDir := filepath.Join(homeDir, ".node-data", "shards")

	if err := os.MkdirAll(shardsDir, 0700); err != nil {
		writeResponse(writer, false)
		return
	}

	shardPath := filepath.Join(shardsDir, shardName)

	if err := os.WriteFile(shardPath, shard, 0600); err != nil {
		writeResponse(writer, false)
		return
	}

	if err := storage.Put(db, "inventory", []byte(shardName), []byte(shardPath)); err != nil {
		os.Remove(shardPath)
		writeResponse(writer, false)
		return
	}

	writeResponse(writer, true)
}

func handlePutCatalog(stream network.Stream, db *bbolt.DB) {
	defer stream.Close()

	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	log.Printf("PUT_CATALOG: incoming stream from %s", stream.Conn().RemotePeer())

	objectName, err := readString(reader)
	if err != nil {
		log.Printf("PUT_CATALOG: failed to read object name: %v", err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: object name: %s", objectName)

	objectType, err := reader.ReadByte()
	if err != nil {
		log.Printf("PUT_CATALOG: failed to read object type: %v", err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: object type: %d", objectType)

	ed25519PublicKey := make([]byte, 32)

	if _, err := io.ReadFull(reader, ed25519PublicKey); err != nil {
		log.Printf("PUT_CATALOG: failed to read public key: %v", err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: received Ed25519 public key")

	var catalogSize uint64

	if err := binary.Read(reader, binary.BigEndian, &catalogSize); err != nil {
		log.Printf("PUT_CATALOG: failed to read catalog size: %v", err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: catalog size: %d bytes", catalogSize)

	if catalogSize > uint64(^uint(0)>>1) {
		log.Printf("PUT_CATALOG: catalog too large: %d bytes", catalogSize)
		writeResponse(writer, false)
		return
	}

	catalog := make([]byte, int(catalogSize))

	if _, err := io.ReadFull(reader, catalog); err != nil {
		log.Printf("PUT_CATALOG: failed to read catalog data: %v", err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: catalog data received")

	if err := validateCatalogObjectName(objectName); err != nil {
		log.Printf("PUT_CATALOG: invalid object name %q: %v", objectName, err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: object name validated")

	if err := cat.VerifyCatalogObject(objectType, catalog, ed25519PublicKey); err != nil {
		log.Printf("PUT_CATALOG: catalog verification failed: %v", err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: catalog verification successful")

	if err := storage.Put(db, "catalogs", []byte(objectName), catalog); err != nil {
		log.Printf("PUT_CATALOG: failed to store catalog %q: %v", objectName, err)
		writeResponse(writer, false)
		return
	}

	log.Printf("PUT_CATALOG: catalog %q stored successfully", objectName)

	if err := writeResponse(writer, true); err != nil {
		log.Printf("PUT_CATALOG: failed to send success response: %v", err)
		return
	}

	log.Printf("PUT_CATALOG: completed successfully for %q", objectName)
}

func validShardName(name string) bool {
	if len(name) != 66 {
		return false
	}

	for _, c := range name {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}

	return true
}

func validateCatalogObjectName(name string) error {
	if name == "" {
		return fmt.Errorf("empty catalog object name")
	}

	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid catalog object name")
	}

	for _, c := range name {
		if c < 0x20 || c == 0x7f {
			return fmt.Errorf("invalid catalog object name")
		}
	}

	return nil
}

func writeResponse(writer *bufio.Writer, success bool) error {
	if success {
		if err := writer.WriteByte(1); err != nil {
			return err
		}
	} else {
		if err := writer.WriteByte(0); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func handleGetCatalog(stream network.Stream, db *bbolt.DB) {
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(60 * time.Second))

	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	keyHex, err := readString(reader)
	if err != nil || len(keyHex) != 64 {
		writeResponse(writer, false)
		return
	}
	if _, err := hex.DecodeString(keyHex); err != nil {
		writeResponse(writer, false)
		return
	}

	var names []string
	var blobs [][]byte

	err = db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("catalogs"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			parts := strings.Split(string(k), ":")
			if len(parts) >= 3 && parts[1] == keyHex {
				names = append(names, string(k))
				blobs = append(blobs, append([]byte(nil), v...))
			}
			return nil
		})
	})
	if err != nil {
		writeResponse(writer, false)
		return
	}

	if err := writer.WriteByte(1); err != nil {
		return
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(names))); err != nil {
		return
	}
	for i := range names {
		if err := writeString(writer, names[i]); err != nil {
			return
		}
		if err := binary.Write(writer, binary.BigEndian, uint64(len(blobs[i]))); err != nil {
			return
		}
		if _, err := writer.Write(blobs[i]); err != nil {
			return
		}
	}
	if err := writer.Flush(); err != nil {
		log.Printf("GET_CATALOG: flush: %v", err)
	}
}

func handleGetShard(stream network.Stream, db *bbolt.DB) {
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(60 * time.Second))

	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	shardName, err := readString(reader)
	if err != nil {
		writeResponse(writer, false)
		return
	}

	if !validShardName(shardName) {
		writeResponse(writer, false)
		return
	}

	var shardPath string

	err = db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("inventory"))
		if b == nil {
			return fmt.Errorf("inventory bucket not found")
		}

		value := b.Get([]byte(shardName))
		if value == nil {
			return fmt.Errorf("shard not found")
		}

		shardPath = string(value)
		return nil
	})
	if err != nil {
		writeResponse(writer, false)
		return
	}

	shard, err := os.ReadFile(shardPath)
	if err != nil {
		writeResponse(writer, false)
		return
	}

	if err := writer.WriteByte(1); err != nil {
		return
	}

	if err := binary.Write(writer, binary.BigEndian, uint64(len(shard))); err != nil {
		return
	}

	if _, err := writer.Write(shard); err != nil {
		return
	}

	if err := writer.Flush(); err != nil {
		log.Printf("GET_SHARD: flush: %v", err)
	}
}
