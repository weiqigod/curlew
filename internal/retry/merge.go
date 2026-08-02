package retry

// MergeConfigs merges overlay on top of base. Non-nil fields in overlay replace base.
// Object fields (RetryOn, DoNotRetryOn) are deep-merged: non-nil sub-fields replace.
// Array fields within objects replace entirely (not appended).
func MergeConfigs(base, overlay FullConfig) FullConfig {
	result := base

	if overlay.Enabled != nil {
		result.Enabled = overlay.Enabled
	}
	if overlay.MaxAttempts != nil {
		result.MaxAttempts = overlay.MaxAttempts
	}
	if overlay.BackoffStrategy != nil {
		result.BackoffStrategy = overlay.BackoffStrategy
	}
	if overlay.InitialDelayMs != nil {
		result.InitialDelayMs = overlay.InitialDelayMs
	}
	if overlay.MaxDelayMs != nil {
		result.MaxDelayMs = overlay.MaxDelayMs
	}
	if overlay.Jitter != nil {
		result.Jitter = overlay.Jitter
	}
	if overlay.JitterFactor != nil {
		result.JitterFactor = overlay.JitterFactor
	}
	if overlay.RespectRetryAfter != nil {
		result.RespectRetryAfter = overlay.RespectRetryAfter
	}
	if overlay.RetryOn != nil {
		result.RetryOn = mergeRetryOn(result.RetryOn, overlay.RetryOn)
	}
	if overlay.DoNotRetryOn != nil {
		result.DoNotRetryOn = mergeDoNotRetryOn(result.DoNotRetryOn, overlay.DoNotRetryOn)
	}

	return result
}

// MergeAll merges configs left-to-right (later = higher precedence).
func MergeAll(configs ...FullConfig) FullConfig {
	var result FullConfig
	for _, c := range configs {
		result = MergeConfigs(result, c)
	}
	return result
}

func mergeRetryOn(base, overlay *RetryOnConfig) *RetryOnConfig {
	if base == nil {
		cp := *overlay
		return &cp
	}
	result := *base
	if overlay.StatusCodes != nil {
		result.StatusCodes = overlay.StatusCodes
	}
	if overlay.StatusRanges != nil {
		result.StatusRanges = overlay.StatusRanges
	}
	if overlay.NetworkErrors != nil {
		result.NetworkErrors = overlay.NetworkErrors
	}
	if overlay.Timeouts != nil {
		result.Timeouts = overlay.Timeouts
	}
	if overlay.Methods != nil {
		result.Methods = overlay.Methods
	}
	return &result
}

func mergeDoNotRetryOn(base, overlay *DoNotRetryOnConfig) *DoNotRetryOnConfig {
	if base == nil {
		cp := *overlay
		return &cp
	}
	result := *base
	if overlay.StatusCodes != nil {
		result.StatusCodes = overlay.StatusCodes
	}
	if overlay.StatusRanges != nil {
		result.StatusRanges = overlay.StatusRanges
	}
	if overlay.Methods != nil {
		result.Methods = overlay.Methods
	}
	return &result
}
