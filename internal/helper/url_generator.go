package helper

import "time"

type URLGenerator interface {
	GetPublicURL(path string) string
	GetPresignedURL(path string, expiry time.Duration) (string, error)
}
