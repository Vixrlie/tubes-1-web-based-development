package access

import "context"

// Action = permission level buat seorang user
// Urutan dari least - most privileged: view < query < migrate
type Action string

const (
	// Ngeliat skema
	ActionView Action = "view"
	// Jalanin query
	ActionQuery Action = "query"
	// Pake migration
	ActionMigrate Action = "migrate"
)

// Deduct apakah seorang user boleh pake actionnya
type Policy interface {
	Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error)
}

// Allow permission ke semua req
type AllowAll struct{}

func (AllowAll) Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error) {
	return true, nil
}
