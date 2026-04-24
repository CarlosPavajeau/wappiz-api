package codes

// URN is a string type for error code constants
type URN string

const (
	ErrorsNotFound        URN = "err:user:not_found"
	ErrorsUnauthorized    URN = "err:user:unauthorized"
	ErrorsForbidden       URN = "err:user:forbidden"
	ErrorsConflict        URN = "err:user:conflict"
	ErrorsTooManyRequests URN = "err:user:too_many_requests"
	ErrorsBadRequest      URN = "err:user:bad_request"

	// ErrorsForbiddenResourceQuotaExceeded indicates the tenant has exceeded their resource quota for the requested operation.
	ErrorsForbiddenResourceQuotaExceeded URN = "err:user:forbidden:resource_quota_exceeded"
)
