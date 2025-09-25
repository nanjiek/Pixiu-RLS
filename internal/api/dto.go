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
