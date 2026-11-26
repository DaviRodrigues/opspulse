package domain

import (
	"errors"
	"fmt"
)

var (
	ErrServiceDown       = errors.New("service unavaible")
	ErrDiscordAuth       = errors.New("fail to authenticate on discord")
	ErrNotFound          = errors.New("item not found")
	ErrInvalidInterval   = errors.New("interval format invalid. Example to use: 30m or 5s")
	ErrInvalidFileFormat = errors.New("this file loader format don't exist")
	ErrInvalidInput      = errors.New("invalid input")
	ErrAlreadyExists     = errors.New("item already exists")
	ErrDuplicateURL      = fmt.Errorf("%w: duplicate url", ErrAlreadyExists)
)
