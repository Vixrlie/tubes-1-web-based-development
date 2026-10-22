package access

import "context"

// allowAll hanya untuk test, jangan dipakai di kode produksi.
type allowAll struct{}

var _ Policy = allowAll{}

func (allowAll) Can(ctx context.Context, userID int64, connID int64, a Action) (bool, error) {
	return true, nil
}
