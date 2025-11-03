package pagination

const (
	// DefaultLimit is the default number of items returned per page
	DefaultLimit = 50

	// MaxLimit is the maximum number of items allowed per page
	MaxLimit = 1000

	// AdminDefaultLimit is the default limit for admin operations
	AdminDefaultLimit = 100

	// AdminMaxLimit is the maximum limit for admin operations
	AdminMaxLimit = 10000

	// MessageDefaultLimit is the default limit for message queries
	MessageDefaultLimit = 25

	// ThreadDefaultLimit is the default limit for thread queries
	ThreadDefaultLimit = 50
)
