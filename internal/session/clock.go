package session

import "time"

// Clock memungkinkan waktu diinjeksi
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
