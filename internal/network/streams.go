package network

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"io"
	"strings"
	"time"
)

const PutShardProtocol = "/quailfs/put-shard/1.0.0"
const PutCatalogProtocol = "/quailfs/put-catalog/1.0.0"
const GetShardProtocol = "/quailfs/get-shard/1.0.0"
const GetCatalogProtocol = "/quailfs/get-catalog/1.0.0"

const (
	maxCatalogObjects    = 4096
	maxCatalogObjectSize = 64 << 20
)

type CatalogBlob struct {
	Name string
	Type uint8
	Data []byte
}

func PutShard(ctx context.Context, h host.Host, peerID peer.ID, shardName string, shard []byte) error {
	stream, err := h.NewStream(ctx, peerID, PutShardProtocol)
	if err != nil {
		return fmt.Errorf("open PUT_SHARD stream: %w", err)
	}
	defer stream.Close()

	writer := bufio.NewWriter(stream)
	reader := bufio.NewReader(stream)

	if err := writeString(writer, shardName); err != nil {
		return fmt.Errorf("write shard name: %w", err)
	}

	if err := binary.Write(writer, binary.BigEndian, uint64(len(shard))); err != nil {
		return fmt.Errorf("write shard size: %w", err)
	}

	if _, err := writer.Write(shard); err != nil {
		return fmt.Errorf("write shard: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush shard: %w", err)
	}

	success, err := readResponse(reader)
	if err != nil {
		return fmt.Errorf("read PUT_SHARD response: %w", err)
	}

	if !success {
		return fmt.Errorf("peer rejected shard")
	}

	return nil
}

func writeString(writer *bufio.Writer, value string) error {
	if err := binary.Write(
		writer,
		binary.BigEndian,
		uint64(len(value)),
	); err != nil {
		return err
	}

	_, err := writer.WriteString(value)
	return err
}

func readResponse(reader *bufio.Reader) (bool, error) {
	status, err := reader.ReadByte()
	if err != nil {
		return false, err
	}

	switch status {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("invalid response status: %d", status)
	}
}

func PutCatalog(ctx context.Context, h host.Host, peerID peer.ID, objectName string, objectType uint8, catalog []byte, ed25519PublicKey []byte) error {
	stream, err := h.NewStream(ctx, peerID, PutCatalogProtocol)
	if err != nil {
		return fmt.Errorf("open PUT_CATALOG stream: %w", err)
	}
	defer stream.Close()

	writer := bufio.NewWriter(stream)
	reader := bufio.NewReader(stream)

	if err := writeString(writer, objectName); err != nil {
		return fmt.Errorf("write catalog object name: %w", err)
	}

	if err := writer.WriteByte(objectType); err != nil {
		return fmt.Errorf("write catalog object type: %w", err)
	}

	if len(ed25519PublicKey) != 32 {
		return fmt.Errorf("invalid Ed25519 public key length: %d", len(ed25519PublicKey))
	}

	if _, err := writer.Write(ed25519PublicKey); err != nil {
		return fmt.Errorf("write Ed25519 public key: %w", err)
	}

	if err := binary.Write(writer, binary.BigEndian, uint64(len(catalog))); err != nil {
		return fmt.Errorf("write catalog size: %w", err)
	}

	if _, err := writer.Write(catalog); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush catalog: %w", err)
	}

	success, err := readResponse(reader)
	if err != nil {
		return fmt.Errorf("read PUT_CATALOG response: %w", err)
	}

	if !success {
		return fmt.Errorf("peer rejected catalog")
	}

	return nil
}

func readString(reader *bufio.Reader) (string, error) {
	var length uint64

	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return "", err
	}

	if length > uint64(^uint(0)>>1) {
		return "", fmt.Errorf("string too large")
	}

	data := make([]byte, int(length))

	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}

	return string(data), nil
}

func GetShard(ctx context.Context, h host.Host, peerID peer.ID, shardName string) ([]byte, error) {
	stream, err := h.NewStream(ctx, peerID, GetShardProtocol)
	if err != nil {
		return nil, fmt.Errorf("open GET_SHARD stream: %w", err)
	}
	defer stream.Close()

	writer := bufio.NewWriter(stream)
	reader := bufio.NewReader(stream)

	if err := writeString(writer, shardName); err != nil {
		return nil, fmt.Errorf("write shard name: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("flush shard request: %w", err)
	}

	success, err := readResponse(reader)
	if err != nil {
		return nil, fmt.Errorf("read GET_SHARD response: %w", err)
	}

	if !success {
		return nil, fmt.Errorf("peer rejected shard request")
	}

	var size uint64

	if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
		return nil, fmt.Errorf("read shard size: %w", err)
	}

	if size > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("shard too large")
	}

	shard := make([]byte, int(size))

	if _, err := io.ReadFull(reader, shard); err != nil {
		return nil, fmt.Errorf("read shard: %w", err)
	}

	return shard, nil
}

func catalogTypeFromName(name string) uint8 {
	switch strings.SplitN(name, ":", 2)[0] {
	case "userindex":
		return 1
	case "head":
		return 2
	case "manifest":
		return 4
	}
	return 0
}

func GetCatalog(ctx context.Context, h host.Host, peerID peer.ID, keyHex string) ([]CatalogBlob, error) {
	stream, err := h.NewStream(ctx, peerID, GetCatalogProtocol)
	if err != nil {
		return nil, fmt.Errorf("open GET_CATALOG stream: %w", err)
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(60 * time.Second))

	writer := bufio.NewWriter(stream)
	reader := bufio.NewReader(stream)

	if err := writeString(writer, keyHex); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("flush request: %w", err)
	}

	ok, err := readResponse(reader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("peer rejected request")
	}

	var count uint32
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil {
		return nil, fmt.Errorf("read count: %w", err)
	}
	if count > maxCatalogObjects {
		return nil, fmt.Errorf("too many catalog objects: %d", count)
	}

	blobs := make([]CatalogBlob, 0, count)
	for i := uint32(0); i < count; i++ {
		name, err := readString(reader)
		if err != nil {
			return nil, fmt.Errorf("read object name: %w", err)
		}

		var size uint64
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
			return nil, fmt.Errorf("read object size: %w", err)
		}
		if size > maxCatalogObjectSize {
			return nil, fmt.Errorf("catalog object too large: %d", size)
		}

		data := make([]byte, size)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, fmt.Errorf("read object: %w", err)
		}

		blobs = append(blobs, CatalogBlob{
			Name: name,
			Type: catalogTypeFromName(name),
			Data: data,
		})
	}

	return blobs, nil
}
