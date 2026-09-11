package utils

import "errors"

var ErrCacheMiss = errors.New("cache miss")
var ErrCacheMalformed = errors.New("cache malformed")
var ErrMissingDeadline = errors.New("requires a context with a deadline")
var ErrSingleFlightValue = errors.New("unexpected singleflight value type")
