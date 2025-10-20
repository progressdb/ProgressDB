package store

// true if likely contains JSON object/array based on first non-whitespace
func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// exported version of likelyJSON
func LikelyJSON(b []byte) bool { return likelyJSON(b) }
