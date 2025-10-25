package keys

const (
	// notation dictionary for key formats:
	// t   = thread
	// m   = message
	// v   = version
	// idx = index
	// ms  = messages
	// u   = user
	// p   = participant
	// del = soft delete marker
	// rel = relationship marker
	// All keys are lowercase; segments are separated by ":"
	// <...> = variable segment (e.g. <thread_id>, <msg_id>)

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

	// soft delete markers
	SoftDeleteMarker = "del:%s" // del:<original_key>

	// relationship markers
	RelUserOwnsThread = "rel:u:%s:t:%s" // rel:u:<user_id>:t:<thread_id>
	RelThreadHasUser  = "rel:t:%s:u:%s" // rel:t:<thread_id>:u:<user_id>

	// padding widths (fixed for lexicographic ordering)
	TSPadWidth  = 20 // e.g. %020d
	SeqPadWidth = 6  // e.g. %06d
)
