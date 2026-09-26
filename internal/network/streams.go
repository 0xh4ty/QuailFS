package network

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"io"
)

const PutShardProtocol = "/quailfs/put-shard/1.0.0"
const PutCatalogProtocol = "/quailfs/put-catalog/1.0.0"

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
