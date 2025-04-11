package restream

import "errors"

var ErrUnknownProcess = errors.New("unknown process")
var ErrProcessExists = errors.New("process already exists")
var ErrInvalidProcessConfig = errors.New("invalid process config")
var ErrMetadataKeyNotFound = errors.New("unknown metadata key")
var ErrMetadataKeyRequired = errors.New("a key for storing metadata is required")
