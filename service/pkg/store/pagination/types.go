package pagination

import "encoding/base64"

type PaginationRequest struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type PaginationResponse struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
}

func EncodeCursor(key string) string {
	if key == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(key))
}

func DecodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
