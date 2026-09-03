package errs

import "errors"

var (
	ErrServiceDown = errors.New("service unavaible")
	ErrDiscordAuth = errors.New("fail to authenticate on discord")
	ErrConfigNotFound = errors.New("required variable not found")
	ErrInvalidInterval = errors.New("interval format invalid. Example to use: 30m or 5s")
)