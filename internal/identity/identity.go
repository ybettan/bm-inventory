package identity

import (
	"context"
	"fmt"

	"github.com/filanov/bm-inventory/pkg/auth"

	"github.com/thoas/go-funk"
)

var ADMIN_LIST = []string{"ercohen", "mfilanov", "rfreiman", "alazar"}

func GetOwner(ctx context.Context) string {
	username := ctx.Value(auth.ContextUsernameKey)
	if username == nil {
		username = ""
	}
	return username.(string)
}

func GetOwnerFilter(ctx context.Context) string {
	query := ""
	username := GetOwner(ctx)
	if username != "" && !funk.ContainsString(ADMIN_LIST, username) {
		query = fmt.Sprintf("owner = '%s'", username)
	}
	return query
}
