package schema

import (
	"github.com/google/uuid"
)

func newUUIDv7() uuid.UUID {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u
}
