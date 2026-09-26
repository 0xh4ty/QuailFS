package types

type Head struct {
	Body HeadBody
	Sig  Sig
}

type HeadBody struct {
	Schema     uint64
	UserID     []byte
	DatasetID  []byte
	Generation uint64
	ManifestID []byte
	CreatedAt  string
}
