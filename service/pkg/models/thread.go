package models

type Thread struct {
	Key       string   `json:"key"`
	Title     string   `json:"title,omitempty"`
	Author    string   `json:"author"`
	CreatedTS int64    `json:"created_ts,omitempty"`
	UpdatedTS int64    `json:"updated_ts,omitempty"`
	Deleted   bool     `json:"deleted,omitempty"`
	KMS       *KMSMeta `json:"kms,omitempty"`
}

type KMSMeta struct {
	KeyID      string `json:"key_id,omitempty"`
	WrappedDEK string `json:"wrapped_dek,omitempty"`
	KEKID      string `json:"kek_id,omitempty"`
	KEKVersion string `json:"kek_version,omitempty"`
}

func (t *Thread) WithKMS(meta KMSMeta) {
	t.KMS = &meta
}
