package models

import "errors"

// ErrQueueFull indicates an event couldn't be enqueued because the write
// queue was at capacity. Callers use this to distinguish a dropped event
// from a real storage failure.
var ErrQueueFull = errors.New("write queue full")
