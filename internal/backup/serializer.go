package backup

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/0xh4ty/quailfs/pkg/types"
)

func serializeQChunks(qchunks []types.QChunk) []byte {
	count := 0
	var serializedQChunks []byte

	for count < len(qchunks) {
		var buf bytes.Buffer

		offsetBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(offsetBytes, qchunks[count].Offset)
		buf.Write(offsetBytes)

		lengthBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lengthBytes, qchunks[count].Length)
		buf.Write(lengthBytes)

		clengthBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(clengthBytes, qchunks[count].CLength)
		buf.Write(clengthBytes)

		buf.Write(qchunks[count].Data)
		buf.Write(qchunks[count].ChunkID)

		serializedQChunks = append(serializedQChunks, buf.Bytes()...)

		count++
	}

	return serializedQChunks
}

func SerializePackedPlainCollection(packedPlainCollection []types.PackedPlain) [][]byte {
	count := 0
	var serializedPackedPlainCollection [][]byte

	for count < len(packedPlainCollection) {
		var buf bytes.Buffer
		buf.Write([]byte(packedPlainCollection[count].Version))

		payloadLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(payloadLenBytes, packedPlainCollection[count].PayloadLen)
		buf.Write(payloadLenBytes)

		serializedQChunks := serializeQChunks(packedPlainCollection[count].QChunks)
		buf.Write(serializedQChunks)

		buf.Write(packedPlainCollection[count].Padding)

		serializedPackedPlainCollection = append(serializedPackedPlainCollection, buf.Bytes())

		count++
	}

	return serializedPackedPlainCollection
}

func deserializeQChunks(serializedQChunks []byte) []types.QChunk {
	var qchunks []types.QChunk
	position := 0
	for position < len(serializedQChunks) {
		var qchunk types.QChunk
		qchunk.Offset = binary.BigEndian.Uint64(serializedQChunks[position : position+8])
		position += 8
		qchunk.Length = binary.BigEndian.Uint64(serializedQChunks[position : position+8])
		position += 8
		qchunk.CLength = binary.BigEndian.Uint64(serializedQChunks[position : position+8])
		position += 8
		qchunk.Data = serializedQChunks[position : position+int(qchunk.CLength)]
		position += int(qchunk.CLength)
		qchunk.ChunkID = serializedQChunks[position : position+32]
		position += 32
		qchunks = append(qchunks, qchunk)
	}
	return qchunks
}

func DeserializePackedPlainCollection(serializedPackedPlainCollection [][]byte) []types.PackedPlain {
	var packedPlainCollection []types.PackedPlain
	capacity := 2 * 1024 * 1024

	for i := range len(serializedPackedPlainCollection) {
		position := 0
		serializedPackedPlain := serializedPackedPlainCollection[i]
		for position < len(serializedPackedPlain) {
			var packedPlain types.PackedPlain
			packedPlain.Version = string(serializedPackedPlain[position : position+2])
			position += 2
			packedPlain.PayloadLen = binary.BigEndian.Uint64(serializedPackedPlain[position : position+8])
			position += 8
			qchunksLen := int(packedPlain.PayloadLen) - 2 - 8
			packedPlain.QChunks = deserializeQChunks(serializedPackedPlain[position : position+qchunksLen])
			position += qchunksLen
			paddingSize := capacity - int(packedPlain.PayloadLen)
			packedPlain.Padding = serializedPackedPlain[position : position+paddingSize]
			position += paddingSize

			packedPlainCollection = append(packedPlainCollection, packedPlain)
		}
	}

	return packedPlainCollection
}

func SerializeWrappedDatasetBody(wrappedDataset types.WrappedDataset) []byte {
	var serializedWrappedDatasetBody []byte

	var buf bytes.Buffer
	buf.Write(wrappedDataset.Body.UserID)
	buf.Write(wrappedDataset.Body.DatasetID)

	labelLen := uint64(len([]byte(wrappedDataset.Body.Label)))
	labelLenBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(labelLenBytes, labelLen)
	buf.Write(labelLenBytes)
	buf.Write([]byte(wrappedDataset.Body.Label))

	generationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(generationBytes, wrappedDataset.Body.Generation)
	buf.Write(generationBytes)

	buf.Write(wrappedDataset.Body.UserX25519Pubkey)
	buf.Write(wrappedDataset.Body.EphX25519Pubkey)

	encDatasetKeyLen := uint64(len([]byte(wrappedDataset.Body.EncDatasetKey)))
	encDatasetKeyLenBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(encDatasetKeyLenBytes, encDatasetKeyLen)
	buf.Write(encDatasetKeyLenBytes)
	buf.Write(wrappedDataset.Body.EncDatasetKey)

	serializedWrappedDatasetBody = buf.Bytes()

	return serializedWrappedDatasetBody
}

func SerializeDatasetEntries(datasets []types.DatasetEntry) []byte {
	var serializedDatasetEntries []byte
	var buf bytes.Buffer

	datasetEntryCount := uint64(len(datasets))
	datasetEntryCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(datasetEntryCountBytes, datasetEntryCount)
	buf.Write(datasetEntryCountBytes)

	for i := range datasets {
		buf.Write(datasets[i].DatasetID)

		labelLen := uint64(len([]byte(datasets[i].Label)))
		labelLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(labelLenBytes, labelLen)
		buf.Write(labelLenBytes)

		buf.Write([]byte(datasets[i].Label))

		generationBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(generationBytes, datasets[i].Generation)
		buf.Write(generationBytes)
	}

	serializedDatasetEntries = buf.Bytes()

	return serializedDatasetEntries
}

func SerializeUserIndexBody(userIndex types.UserIndex) []byte {
	var serializedUserIndexBody []byte

	var buf bytes.Buffer
	schemaBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(schemaBytes, userIndex.Body.Schema)
	buf.Write(schemaBytes)

	buf.Write(userIndex.Body.UserID)

	generationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(generationBytes, userIndex.Body.Generation)
	buf.Write(generationBytes)

	serializedDatasetEntries := SerializeDatasetEntries(userIndex.Body.Datasets)
	buf.Write(serializedDatasetEntries)

	updatedAtBytes := []byte(userIndex.Body.UpdatedAt)

	updatedAtLengthBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(updatedAtLengthBytes, uint64(len(updatedAtBytes)))
	buf.Write(updatedAtLengthBytes)

	buf.Write(updatedAtBytes)

	serializedUserIndexBody = buf.Bytes()

	return serializedUserIndexBody
}

func SerializeCatalogPeerHints(catalogPeerHints [][]byte) []byte {
	var serializedCatalogPeerHints []byte

	var buf bytes.Buffer

	for i := range catalogPeerHints {
		buf.Write(catalogPeerHints[i])
	}

	serializedCatalogPeerHints = buf.Bytes()

	return serializedCatalogPeerHints
}

func SerializeHeadBody(head types.Head) []byte {
	var serializedHeadBody []byte

	var buf bytes.Buffer
	schemaBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(schemaBytes, head.Body.Schema)
	buf.Write(schemaBytes)

	buf.Write(head.Body.UserID)
	buf.Write(head.Body.DatasetID)

	generationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(generationBytes, head.Body.Generation)
	buf.Write(generationBytes)

	buf.Write(head.Body.ManifestID)

	createdAtBytes := []byte(head.Body.CreatedAt)

	createdAtLengthBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(createdAtLengthBytes, uint64(len(createdAtBytes)))
	buf.Write(createdAtLengthBytes)

	buf.Write(createdAtBytes)

	serializedHeadBody = buf.Bytes()

	return serializedHeadBody
}

func SerializeFiles(files []types.File) []byte {
	var serializedFiles []byte

	var buf bytes.Buffer

	filesCount := uint64(len(files))
	filesCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(filesCountBytes, filesCount)
	buf.Write(filesCountBytes)

	for i := range files {
		fileNameLen := uint64(len([]byte(files[i].FileName)))
		fileNameLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(fileNameLenBytes, fileNameLen)
		buf.Write(fileNameLenBytes)

		buf.Write([]byte(files[i].FileName))

		chunkIDCount := uint64(len(files[i].ChunkIDs))
		chunkIDCountBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(chunkIDCountBytes, chunkIDCount)
		buf.Write(chunkIDCountBytes)

		for j := range files[i].ChunkIDs {
			buf.Write(files[i].ChunkIDs[j])
		}
	}

	serializedFiles = buf.Bytes()

	return serializedFiles
}

func SerializeTrees(trees []types.Tree) []byte {
	var buf bytes.Buffer

	treeCount := uint64(len(trees))
	treeCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(treeCountBytes, treeCount)
	buf.Write(treeCountBytes)

	for _, tree := range trees {
		rootDirectoryLen := uint64(len([]byte(tree.RootDirectory)))
		rootDirectoryLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(rootDirectoryLenBytes, rootDirectoryLen)
		buf.Write(rootDirectoryLenBytes)

		buf.Write([]byte(tree.RootDirectory))

		serializedFiles := SerializeFiles(tree.Files)
		buf.Write(serializedFiles)

		childDirectoryCount := uint64(len(tree.ChildDirectories))
		childDirectoryCountBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(childDirectoryCountBytes, childDirectoryCount)
		buf.Write(childDirectoryCountBytes)

		for _, childDirectory := range tree.ChildDirectories {
			serializedChildDirectory := SerializeTrees([]types.Tree{*childDirectory})
			buf.Write(serializedChildDirectory)
		}
	}

	return buf.Bytes()
}

func SerializeMChunks(mchunks []types.MChunk) []byte {
	var serializedMChunks []byte

	var buf bytes.Buffer

	mchunksCount := uint64(len(mchunks))
	mchunksCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(mchunksCountBytes, mchunksCount)
	buf.Write(mchunksCountBytes)

	for i := range mchunks {
		buf.Write(mchunks[i].ChunkID)

		sizeBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(sizeBytes, mchunks[i].Size)
		buf.Write(sizeBytes)

		buf.Write(mchunks[i].StripeID)
	}

	serializedMChunks = buf.Bytes()

	return serializedMChunks
}

func SerializeMStripes(mstripes []types.MStripe) []byte {
	var serializedMStripes []byte

	var buf bytes.Buffer

	mstripesCount := uint64(len(mstripes))
	mstripesCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(mstripesCountBytes, mstripesCount)
	buf.Write(mstripesCountBytes)

	for i := range mstripes {
		buf.Write(mstripes[i].StripeID)

		kBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(kBytes, mstripes[i].K)
		buf.Write(kBytes)

		nBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(nBytes, mstripes[i].N)
		buf.Write(nBytes)

		PayloadLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(PayloadLenBytes, mstripes[i].PayloadLen)
		buf.Write(PayloadLenBytes)

		shardNamesCount := uint64(len(mstripes[i].ShardNames))
		shardNamesCountBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(shardNamesCountBytes, shardNamesCount)
		buf.Write(shardNamesCountBytes)

		for j := range mstripes[i].ShardNames {
			buf.Write(mstripes[i].ShardNames[j])
		}
	}

	serializedMStripes = buf.Bytes()

	return serializedMStripes
}

func SerializeTombstones(tombstones []string) []byte {
	var serializedTombstones []byte

	var buf bytes.Buffer

	tombstonesCount := uint64(len(tombstones))
	tombstonesCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(tombstonesCountBytes, tombstonesCount)
	buf.Write(tombstonesCountBytes)

	for i := range tombstones {
		tombstoneLen := uint64(len([]byte(tombstones[i])))
		tombstoneLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(tombstoneLenBytes, tombstoneLen)
		buf.Write(tombstoneLenBytes)

		buf.Write([]byte(tombstones[i]))
	}

	serializedTombstones = buf.Bytes()

	return serializedTombstones
}

func SerializeManifestPlain(manifest types.ManifestEnvelope) []byte {
	var serializedManifestPlain []byte

	var buf bytes.Buffer
	buf.Write(manifest.ManifestPlain.DatasetID)

	generationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(generationBytes, manifest.ManifestPlain.Generation)
	buf.Write(generationBytes)

	buf.Write(manifest.ManifestPlain.ParentManifestID)

	serializedTree := SerializeTrees(manifest.ManifestPlain.Trees)
	buf.Write(serializedTree)

	serializedMChunks := SerializeMChunks(manifest.ManifestPlain.MChunks)
	buf.Write(serializedMChunks)

	serializedMStripes := SerializeMStripes(manifest.ManifestPlain.MStripes)
	buf.Write(serializedMStripes)

	serializedTombstones := SerializeTombstones(manifest.ManifestPlain.Tombstones)
	buf.Write(serializedTombstones)

	serializedManifestPlain = buf.Bytes()

	return serializedManifestPlain
}

func SerializeHead(head types.Head) []byte {
	var buf bytes.Buffer

	serializedBody := SerializeHeadBody(head)
	buf.Write(serializedBody)

	buf.Write(head.Sig.Signature)

	return buf.Bytes()
}

func SerializeUserIndex(userIndex types.UserIndex) []byte {
	var buf bytes.Buffer

	serializedBody := SerializeUserIndexBody(userIndex)
	buf.Write(serializedBody)

	buf.Write(userIndex.Sig.Signature)

	return buf.Bytes()
}

func SerializeWrappedDataset(wrappedDataset types.WrappedDataset) []byte {
	var buf bytes.Buffer

	serializedBody := SerializeWrappedDatasetBody(wrappedDataset)
	buf.Write(serializedBody)

	buf.Write(wrappedDataset.Sig.Signature)

	return buf.Bytes()
}

func SerializeManifestEnvelope(manifest types.ManifestEnvelope) []byte {
	var buf bytes.Buffer

	buf.Write(manifest.Envelope)
	buf.Write(manifest.Sig.Signature)
	buf.Write(manifest.ManifestID)

	return buf.Bytes()
}

func DeserializeUserIndex(data []byte) (types.UserIndex, error) {
	var userIndex types.UserIndex
	position := 0

	if len(data) < 8 {
		return types.UserIndex{}, fmt.Errorf("invalid user index: missing schema")
	}

	userIndex.Body.Schema = binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+32 {
		return types.UserIndex{}, fmt.Errorf("invalid user index: missing user ID")
	}

	userIndex.Body.UserID = data[position : position+32]
	position += 32

	if len(data) < position+8 {
		return types.UserIndex{}, fmt.Errorf("invalid user index: missing generation")
	}

	userIndex.Body.Generation = binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+8 {
		return types.UserIndex{}, fmt.Errorf("invalid user index: missing dataset count")
	}

	datasetCount := binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	for range datasetCount {
		if len(data) < position+32 {
			return types.UserIndex{}, fmt.Errorf("invalid user index: missing dataset ID")
		}

		datasetID := data[position : position+32]
		position += 32

		if len(data) < position+8 {
			return types.UserIndex{}, fmt.Errorf("invalid user index: missing label length")
		}

		labelLen := binary.BigEndian.Uint64(data[position : position+8])
		position += 8

		if labelLen > uint64(len(data)-position) {
			return types.UserIndex{}, fmt.Errorf("invalid user index: truncated label")
		}

		label := string(data[position : position+int(labelLen)])
		position += int(labelLen)

		if len(data) < position+8 {
			return types.UserIndex{}, fmt.Errorf("invalid user index: missing dataset generation")
		}

		generation := binary.BigEndian.Uint64(data[position : position+8])
		position += 8

		userIndex.Body.Datasets = append(userIndex.Body.Datasets, types.DatasetEntry{
			DatasetID:  datasetID,
			Label:      label,
			Generation: generation,
		})
	}

	if len(data) < position+8 {
		return types.UserIndex{}, fmt.Errorf("invalid user index: missing updated at length")
	}

	updatedAtLength := binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if updatedAtLength > uint64(len(data)-position) {
		return types.UserIndex{}, fmt.Errorf("invalid user index: truncated updated at")
	}

	userIndex.Body.UpdatedAt = string(data[position : position+int(updatedAtLength)])
	position += int(updatedAtLength)

	if len(data) < position+64 {
		return types.UserIndex{}, fmt.Errorf("invalid user index: missing signature")
	}

	userIndex.Sig.Signature = data[position : position+64]
	position += 64

	return userIndex, nil
}

func DeserializeWrappedDataset(data []byte) (types.WrappedDataset, error) {
	var wrappedDataset types.WrappedDataset
	position := 0

	if len(data) < 32 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing user ID")
	}

	wrappedDataset.Body.UserID = data[position : position+32]
	position += 32

	if len(data) < position+32 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing dataset ID")
	}

	wrappedDataset.Body.DatasetID = data[position : position+32]
	position += 32

	if len(data) < position+8 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing label length")
	}

	labelLen := binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+int(labelLen) {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: truncated label")
	}

	wrappedDataset.Body.Label = string(data[position : position+int(labelLen)])
	position += int(labelLen)

	if len(data) < position+8 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing generation")
	}

	wrappedDataset.Body.Generation = binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+32 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing user X25519 public key")
	}

	wrappedDataset.Body.UserX25519Pubkey = data[position : position+32]
	position += 32

	if len(data) < position+32 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing ephemeral X25519 public key")
	}

	wrappedDataset.Body.EphX25519Pubkey = data[position : position+32]
	position += 32

	if len(data) < position+8 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: missing encrypted key length")
	}

	encDatasetKeyLen := binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+int(encDatasetKeyLen)+64 {
		return types.WrappedDataset{}, fmt.Errorf("invalid wrapped dataset: truncated encrypted key or signature")
	}

	wrappedDataset.Body.EncDatasetKey = data[position : position+int(encDatasetKeyLen)]
	position += int(encDatasetKeyLen)

	wrappedDataset.Sig.Signature = data[position : position+64]

	return wrappedDataset, nil
}

func deserializeHeadWithLength(data []byte) (types.Head, int, error) {
	var head types.Head
	position := 0

	if len(data) < position+8 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing schema")
	}

	head.Body.Schema = binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+32 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing user ID")
	}

	head.Body.UserID = data[position : position+32]
	position += 32

	if len(data) < position+32 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing dataset ID")
	}

	head.Body.DatasetID = data[position : position+32]
	position += 32

	if len(data) < position+8 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing generation")
	}

	head.Body.Generation = binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if len(data) < position+32 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing manifest ID")
	}

	head.Body.ManifestID = data[position : position+32]
	position += 32

	if len(data) < position+8 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing created at length")
	}

	createdAtLength := binary.BigEndian.Uint64(data[position : position+8])
	position += 8

	if createdAtLength > uint64(len(data)-position) {
		return types.Head{}, 0, fmt.Errorf("invalid head: truncated created at")
	}

	head.Body.CreatedAt = string(data[position : position+int(createdAtLength)])
	position += int(createdAtLength)

	if len(data) < position+64 {
		return types.Head{}, 0, fmt.Errorf("invalid head: missing signature")
	}

	head.Sig.Signature = data[position : position+64]
	position += 64

	return head, position, nil
}

func DeserializeHead(data []byte) (types.Head, error) {
	head, _, err := deserializeHeadWithLength(data)
	if err != nil {
		return types.Head{}, err
	}

	return head, nil
}

func DeserializeHeadCatalog(data []byte) (types.Head, types.WrappedDataset, error) {
	head, position, err := deserializeHeadWithLength(data)
	if err != nil {
		return types.Head{}, types.WrappedDataset{}, err
	}

	wrappedDataset, err := DeserializeWrappedDataset(data[position:])
	if err != nil {
		return types.Head{}, types.WrappedDataset{}, fmt.Errorf("deserialize wrapped dataset: %w", err)
	}

	return head, wrappedDataset, nil
}

func DeserializeManifestEnvelope(data []byte) (types.ManifestEnvelope, error) {
	const signatureLen = 64
	const manifestIDLen = 32

	if len(data) < signatureLen+manifestIDLen {
		return types.ManifestEnvelope{}, fmt.Errorf("invalid manifest envelope")
	}

	envelopeEnd := len(data) - signatureLen - manifestIDLen

	var manifest types.ManifestEnvelope

	manifest.Envelope = data[:envelopeEnd]
	manifest.Sig.Signature = data[envelopeEnd : envelopeEnd+signatureLen]
	manifest.ManifestID = data[envelopeEnd+signatureLen:]

	return manifest, nil
}

func DeserializeCatalogObject(objectType uint8, data []byte) error {
	switch objectType {
	case 1:
		_, err := DeserializeUserIndex(data)
		return err

	case 2:
		_, _, err := DeserializeHeadCatalog(data)
		return err

	case 4:
		_, err := DeserializeManifestEnvelope(data)
		return err

	default:
		return fmt.Errorf("unknown catalog object type: %d", objectType)
	}
}

func DeserializeFiles(data []byte) ([]types.File, int, error) {
	offset := 0

	if len(data) < offset+8 {
		return nil, 0, fmt.Errorf("invalid files: missing file count")
	}

	filesCount := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	files := make([]types.File, 0, filesCount)

	for range filesCount {
		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid file: missing filename length")
		}

		fileNameLen := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if fileNameLen > uint64(len(data)-offset) {
			return nil, 0, fmt.Errorf("invalid file: filename exceeds data")
		}

		fileName := string(data[offset : offset+int(fileNameLen)])
		offset += int(fileNameLen)

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid file: missing chunk ID count")
		}

		chunkIDCount := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		chunkIDs := make([][]byte, 0, chunkIDCount)

		for range chunkIDCount {
			if len(data) < offset+32 {
				return nil, 0, fmt.Errorf("invalid file: missing chunk ID")
			}

			chunkID := append([]byte(nil), data[offset:offset+32]...)
			offset += 32

			chunkIDs = append(chunkIDs, chunkID)
		}

		files = append(files, types.File{
			FileName: fileName,
			ChunkIDs: chunkIDs,
		})
	}

	return files, offset, nil
}

func DeserializeTrees(data []byte) ([]types.Tree, int, error) {
	offset := 0

	if len(data) < offset+8 {
		return nil, 0, fmt.Errorf("invalid trees: missing tree count")
	}

	treeCount := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	trees := make([]types.Tree, 0, treeCount)

	for range treeCount {
		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid tree: missing root directory length")
		}

		rootDirectoryLen := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if rootDirectoryLen > uint64(len(data)-offset) {
			return nil, 0, fmt.Errorf("invalid tree: root directory exceeds data")
		}

		rootDirectory := string(data[offset : offset+int(rootDirectoryLen)])
		offset += int(rootDirectoryLen)

		files, consumed, err := DeserializeFiles(data[offset:])
		if err != nil {
			return nil, 0, fmt.Errorf("deserialize files: %w", err)
		}
		offset += consumed

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid tree: missing child directory count")
		}

		childDirectoryCount := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		childDirectories := make([]*types.Tree, 0, childDirectoryCount)

		for range childDirectoryCount {
			childTrees, consumed, err := DeserializeTrees(data[offset:])
			if err != nil {
				return nil, 0, fmt.Errorf("deserialize child tree: %w", err)
			}

			if len(childTrees) != 1 {
				return nil, 0, fmt.Errorf("invalid child tree serialization")
			}

			offset += consumed
			childDirectories = append(childDirectories, &childTrees[0])
		}

		trees = append(trees, types.Tree{
			RootDirectory:    rootDirectory,
			Files:            files,
			ChildDirectories: childDirectories,
		})
	}

	return trees, offset, nil
}

func DeserializeMChunks(data []byte) ([]types.MChunk, int, error) {
	offset := 0

	if len(data) < offset+8 {
		return nil, 0, fmt.Errorf("invalid mchunks: missing count")
	}

	mchunksCount := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	mchunks := make([]types.MChunk, 0, mchunksCount)

	for range mchunksCount {
		if len(data) < offset+32 {
			return nil, 0, fmt.Errorf("invalid mchunk: missing chunk ID")
		}

		chunkID := append([]byte(nil), data[offset:offset+32]...)
		offset += 32

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid mchunk: missing size")
		}

		size := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if len(data) < offset+32 {
			return nil, 0, fmt.Errorf("invalid mchunk: missing stripe ID")
		}

		stripeID := append([]byte(nil), data[offset:offset+32]...)
		offset += 32

		mchunks = append(mchunks, types.MChunk{
			ChunkID:  chunkID,
			Size:     size,
			StripeID: stripeID,
		})
	}

	return mchunks, offset, nil
}

func DeserializeMStripes(data []byte) ([]types.MStripe, int, error) {
	const shardNameSize = 66

	offset := 0

	if len(data) < offset+8 {
		return nil, 0, fmt.Errorf("invalid mstripes: missing count")
	}

	mstripesCount := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	mstripes := make([]types.MStripe, 0, mstripesCount)

	for range mstripesCount {
		if len(data) < offset+32 {
			return nil, 0, fmt.Errorf("invalid mstripe: missing stripe ID")
		}

		stripeID := append([]byte(nil), data[offset:offset+32]...)
		offset += 32

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid mstripe: missing K")
		}

		k := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid mstripe: missing N")
		}

		n := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid mstripe: missing payload length")
		}

		payloadLen := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid mstripe: missing shard names count")
		}

		shardNamesCount := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		shardNames := make([][]byte, 0, shardNamesCount)

		for range shardNamesCount {
			if len(data) < offset+shardNameSize {
				return nil, 0, fmt.Errorf("invalid mstripe: missing shard name")
			}

			shardName := append([]byte(nil), data[offset:offset+shardNameSize]...)
			offset += shardNameSize

			shardNames = append(shardNames, shardName)
		}

		mstripes = append(mstripes, types.MStripe{
			StripeID:   stripeID,
			K:          k,
			N:          n,
			PayloadLen: payloadLen,
			ShardNames: shardNames,
		})
	}

	return mstripes, offset, nil
}

func DeserializeTombstones(data []byte) ([]string, int, error) {
	offset := 0

	if len(data) < offset+8 {
		return nil, 0, fmt.Errorf("invalid tombstones: missing count")
	}

	tombstonesCount := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	tombstones := make([]string, 0, tombstonesCount)

	for range tombstonesCount {
		if len(data) < offset+8 {
			return nil, 0, fmt.Errorf("invalid tombstone: missing length")
		}

		tombstoneLen := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if tombstoneLen > uint64(len(data)-offset) {
			return nil, 0, fmt.Errorf("invalid tombstone: length exceeds data")
		}

		tombstone := string(data[offset : offset+int(tombstoneLen)])
		offset += int(tombstoneLen)

		tombstones = append(tombstones, tombstone)
	}

	return tombstones, offset, nil
}

func DeserializeManifestPlain(data []byte) (types.ManifestPlain, int, error) {
	offset := 0

	if len(data) < offset+32 {
		return types.ManifestPlain{}, 0, fmt.Errorf("invalid manifest: missing dataset ID")
	}

	datasetID := append([]byte(nil), data[offset:offset+32]...)
	offset += 32

	if len(data) < offset+8 {
		return types.ManifestPlain{}, 0, fmt.Errorf("invalid manifest: missing generation")
	}

	generation := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	if len(data) < offset+32 {
		return types.ManifestPlain{}, 0, fmt.Errorf("invalid manifest: missing parent manifest ID")
	}

	parentManifestID := append([]byte(nil), data[offset:offset+32]...)
	offset += 32

	trees, consumed, err := DeserializeTrees(data[offset:])
	if err != nil {
		return types.ManifestPlain{}, 0, fmt.Errorf("deserialize trees: %w", err)
	}
	offset += consumed

	mchunks, consumed, err := DeserializeMChunks(data[offset:])
	if err != nil {
		return types.ManifestPlain{}, 0, fmt.Errorf("deserialize mchunks: %w", err)
	}
	offset += consumed

	mstripes, consumed, err := DeserializeMStripes(data[offset:])
	if err != nil {
		return types.ManifestPlain{}, 0, fmt.Errorf("deserialize mstripes: %w", err)
	}
	offset += consumed

	tombstones, consumed, err := DeserializeTombstones(data[offset:])
	if err != nil {
		return types.ManifestPlain{}, 0, fmt.Errorf("deserialize tombstones: %w", err)
	}
	offset += consumed

	return types.ManifestPlain{
		DatasetID:        datasetID,
		Generation:       generation,
		ParentManifestID: parentManifestID,
		Trees:            trees,
		MChunks:          mchunks,
		MStripes:         mstripes,
		Tombstones:       tombstones,
	}, offset, nil
}
