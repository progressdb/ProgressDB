package kms

import kmss "github.com/progressdb/kms/pkg/security"

// KMSProvider is an alias to the provider interface implemented by the
// kms library so the server and kms packages share the same type.
type KMSProvider = kmss.KMSProvider
