package keys

const (
	// notation dictionary (lowercase for key formats):
	// m: message
	// t: thread
	// v: version
	// idx: index
	// p: participants
	// u: user
	// ms: message store

	// provisional
	ThreadPrvKey  = "t:%s"      // t:<threadID>
	MessagePrvKey = "t:%s:m:%s" // t:<threadID>

	// primary storage key formats
	MessageKey = "t:%s:m:%s:%s" // t:<threadID>:m:<msgID>:<seq>
	VersionKey = "v:%s:%s:%s"   // v:<msgID>:<ts>:<seq>
	ThreadKey  = "t:%s"         // t:<threadID>

	// thread → message indexes
	ThreadMessageStart   = "idx:t:%s:ms:start"   // idx:t:<thread_id>:ms:start
	ThreadMessageEnd     = "idx:t:%s:ms:end"     // idx:t:<thread_id>:ms:end
	ThreadMessageCDeltas = "idx:t:%s:ms:cdeltas" // idx:t:<thread_id>:ms:cdeltas
	ThreadMessageUDeltas = "idx:t:%s:ms:udeltas" // idx:t:<thread_id>:ms:udeltas
	ThreadMessageSkips   = "idx:t:%s:ms:skips"   // idx:t:<thread_id>:ms:skips
	ThreadMessageLC      = "idx:t:%s:ms:lc"      // idx:t:<thread_id>:ms:lc (last created at)
	ThreadMessageLU      = "idx:t:%s:ms:lu"      // idx:t:<thread_id>:ms:lu (last updated at)

	// thread → message version indexes
	ThreadVersionStart   = "idx:t:%s:ms:%s:vs:start"   // idx:t:<thread_id>:ms:<msg_id>:vs:start
	ThreadVersionEnd     = "idx:t:%s:ms:%s:vs:end"     // idx:t:<thread_id>:ms:<msg_id>:vs:end
	ThreadVersionCDeltas = "idx:t:%s:ms:%s:vs:cdeltas" // idx:t:<thread_id>:ms:<msg_id>:vs:cdeltas
	ThreadVersionUDeltas = "idx:t:%s:ms:%s:vs:udeltas" // idx:t:<thread_id>:ms:<msg_id>:vs:udeltas
	ThreadVersionSkips   = "idx:t:%s:ms:%s:vs:skips"   // idx:t:<thread_id>:ms:<msg_id>:vs:skips
	ThreadVersionLC      = "idx:t:%s:ms:%s:vs:lc"      // idx:t:<thread_id>:ms:<msg_id>:vs:lc (last created at for version)
	ThreadVersionLU      = "idx:t:%s:ms:%s:vs:lu"      // idx:t:<thread_id>:ms:<msg_id>:vs:lu (last updated at for version)

	// user → thread indexes (ownership)
	UserThreads = "idx:u:%s:threads" // idx:u:<user_id>:threads

	// thread → participant indexes
	ThreadParticipants = "idx:p:%s" // idx:p:<thread_id>

	// soft delete markers
	SoftDeleteMarker = "del:%s" // del:<original_key>

	// padding widths (fixed for lexicographic ordering)
	TSPadWidth  = 20 // e.g. %020d
	SeqPadWidth = 6  // e.g. %06d
)
