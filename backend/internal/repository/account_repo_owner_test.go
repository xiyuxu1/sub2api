package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestAccountEntityToServicePreservesOwnerUserID(t *testing.T) {
	ownerUserID := int64(42)

	account := accountEntityToService(&dbent.Account{OwnerUserID: &ownerUserID})

	require.NotNil(t, account)
	require.NotNil(t, account.OwnerUserID)
	require.Equal(t, ownerUserID, *account.OwnerUserID)
}
