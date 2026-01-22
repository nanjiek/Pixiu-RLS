package api

type AllowRequest struct {
	RuleID string            `json:"ruleId"`
	Dims   map[string]string `json:"dims"` // ip,userId,appId,route...
}

type AllowResponse struct {
	Allowed      bool   `json:"allowed"`
	Remaining    int64  `json:"remaining"`
	RetryAfterMs int64  `json:"retryAfterMs"`
	Reason       string `json:"reason"`
}

type ErrorDetail struct {
	Reason       string `json:"reason,omitempty"`
	RuleID       string `json:"rule_id,omitempty"`
	RetryAfter   int64  `json:"retry_after,omitempty"`
	BlockedUntil int64  `json:"blocked_until,omitempty"`
}

type ErrorResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Detail  *ErrorDetail `json:"detail,omitempty"`
}
