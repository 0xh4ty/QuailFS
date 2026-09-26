package catalog

import (
	"bytes"
	"crypto/ed25519"
	"github.com/0xh4ty/quailfs/internal/backup"
	"github.com/0xh4ty/quailfs/pkg/types"
	"time"
)

func CreateHead(schema uint64, userID []byte, datasetID []byte, generation uint64, manifestID []byte, ed25519privateKey []byte) types.Head {
	var head types.Head

	head.Body.Schema = schema
	head.Body.UserID = userID
	head.Body.DatasetID = datasetID
	head.Body.Generation = generation
	head.Body.ManifestID = manifestID

	createdAt := time.Now().UTC().Format(time.RFC3339)
	head.Body.CreatedAt = createdAt

	serializeHeadBody := backup.SerializeHeadBody(head)
	var buf bytes.Buffer
	buf.WriteString("quailfs/head/v1")
	buf.Write(serializeHeadBody)

	signature := ed25519.Sign(ed25519privateKey, buf.Bytes())
	head.Sig.Signature = signature

	return head
}
