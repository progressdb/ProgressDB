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
	// <...> = variable segment (e.g. <thread_key>, <msg_id>)

	// provisional
	ThreadPrvKey  = "t:%s"      // t:<threadTS>
	MessagePrvKey = "t:%s:m:%s" // t:<threadTS>

	// primary storage key formats
	MessageKey = "t:%s:m:%s:%s" // t:<threadTS>:m:<messageTS>:<seq>
	VersionKey = "v:%s:%s:%s"   // v:<messageKey>:<ts>:<seq>
	ThreadKey  = "t:%s"         // t:<threadTS>

	// thread → message indexes
	ThreadMessageStart   = "idx:t:%s:ms:start"   // idx:t:<thread_key>:ms:start
	ThreadMessageEnd     = "idx:t:%s:ms:end"     // idx:t:<thread_key>:ms:end
	ThreadMessageCDeltas = "idx:t:%s:ms:cdeltas" // idx:t:<thread_key>:ms:cdeltas
	ThreadMessageUDeltas = "idx:t:%s:ms:udeltas" // idx:t:<thread_key>:ms:udeltas
	ThreadMessageSkips   = "idx:t:%s:ms:skips"   // idx:t:<thread_key>:ms:skips
	ThreadMessageLC      = "idx:t:%s:ms:lc"      // idx:t:<thread_key>:ms:lc (last created at)
	ThreadMessageLU      = "idx:t:%s:ms:lu"      // idx:t:<thread_key>:ms:lu (last updated at)

	// thread → message version indexes
	ThreadVersionStart   = "idx:t:%s:ms:%s:vs:start"   // idx:t:<thread_key>:ms:<msg_id>:vs:start
	ThreadVersionEnd     = "idx:t:%s:ms:%s:vs:end"     // idx:t:<thread_key>:ms:<msg_id>:vs:end
	ThreadVersionCDeltas = "idx:t:%s:ms:%s:vs:cdeltas" // idx:t:<thread_key>:ms:<msg_id>:vs:cdeltas
	ThreadVersionUDeltas = "idx:t:%s:ms:%s:vs:udeltas" // idx:t:<thread_key>:ms:<msg_id>:vs:udeltas
	ThreadVersionSkips   = "idx:t:%s:ms:%s:vs:skips"   // idx:t:<thread_key>:ms:<msg_id>:vs:skips
	ThreadVersionLC      = "idx:t:%s:ms:%s:vs:lc"      // idx:t:<thread_key>:ms:<msg_id>:vs:lc (last created at for version)
	ThreadVersionLU      = "idx:t:%s:ms:%s:vs:lu"      // idx:t:<thread_key>:ms:<msg_id>:vs:lu (last updated at for version)

	// soft delete markers
	SoftDeleteMarker = "del:%s" // del:<original_key>

	// relationship markers
	RelUserOwnsThread = "rel:u:%s:t:%s" // rel:u:<user_id>:t:<thread_key>
	RelThreadHasUser  = "rel:t:%s:u:%s" // rel:t:<thread_key>:u:<user_id>

	// padding widths (fixed for lexicographic ordering)
	TSPadWidth  = 20 // e.g. %020d
	SeqPadWidth = 6  // e.g. %06d

	// system keys
	SystemVersionKey    = "system:version"
	SystemInProgressKey = "system:migration_in_progress"
)
