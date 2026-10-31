// Package ids membuat ID UUIDv7 (urut waktu).
package ids

import "github.com/google/uuid"

func New() uuid.UUID { return uuid.Must(uuid.NewV7()) }
