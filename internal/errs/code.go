package errs

const (
	// 通用错误
	InternalError = 10000
	ParamInvalid  = 10101
	Unauthorized  = 10401
	Forbidden     = 10403
	NotFound      = 10404

	// Middleware错误
	// 鉴权
	JWTExpired   = 20100
	JWTInvalid   = 20101
	TokenExpired = 20102

	// 服务错误
	// Activity
	ActivityNotFound     = 30000
	ActivityExpired      = 30001
	ActivityCreateFailed = 30002
	DraftNotFound        = 30003

	// Post
	PostNotFound     = 31000
	PostCreateFailed = 31001

	// User
	UserNotFound     = 32000
	UserBanned       = 32001
	LoginFailed      = 32202
	NetworkError     = 32203
	RealNameMismatch = 32204
	LoginInfoInvalid = 32205

	// Comment
	CommentNotFound       = 33000
	CommentParentNotFound = 33001
	InvalidSubject        = 33002

	// Interaction
	InteractionSubjectInvalid = 34000

	// Feed
	FeedListFailed = 35000

	// Auditor
	UploadFormFailed = 36000
	CreateFormFailed = 36001
)
