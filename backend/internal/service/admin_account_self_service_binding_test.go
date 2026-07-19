package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/authctx"
	"github.com/stretchr/testify/require"
)

type selfServiceBindingAccountRepo struct {
	AdminAccountRepository
	account       *Account
	boundGroupIDs []int64
}

func (r *selfServiceBindingAccountRepo) Create(_ context.Context, account *Account) error {
	account.ID = 101
	r.account = account
	return nil
}

func (r *selfServiceBindingAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *selfServiceBindingAccountRepo) Update(_ context.Context, account *Account) error {
	r.account = account
	return nil
}

func (r *selfServiceBindingAccountRepo) BindGroups(_ context.Context, _ int64, groupIDs []int64) error {
	r.boundGroupIDs = append([]int64(nil), groupIDs...)
	if r.account != nil {
		r.account.GroupIDs = append([]int64(nil), groupIDs...)
	}
	return nil
}

func (r *selfServiceBindingAccountRepo) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}

type selfServiceBindingGroupRepo struct {
	AdminGroupRepository
}

func (r *selfServiceBindingGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	return &Group{ID: id, Name: "shared", Platform: PlatformOpenAI, Status: StatusActive}, nil
}

func selfServiceActorContext() context.Context {
	return authctx.WithActor(context.Background(), authctx.Actor{UserID: 7, IsAdmin: false})
}

func TestCreateAccountSelfServiceBModeKeepsExplicitProxyAndGroups(t *testing.T) {
	proxyID := int64(12)
	repo := &selfServiceBindingAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: &selfServiceBindingGroupRepo{}}

	created, err := svc.CreateAccount(selfServiceActorContext(), &CreateAccountInput{
		Name:        "mine",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		ProxyID:     &proxyID,
		GroupIDs:    []int64{21, 22},
	})

	require.NoError(t, err)
	require.Equal(t, &proxyID, created.ProxyID)
	require.Equal(t, []int64{21, 22}, repo.boundGroupIDs)
	require.False(t, created.Schedulable, "new self-service accounts still require admin scheduling approval")
}

func TestUpdateAccountSelfServiceBModeKeepsProxyAndGroupChanges(t *testing.T) {
	oldProxyID := int64(12)
	newProxyID := int64(13)
	groupIDs := []int64{31, 32}
	repo := &selfServiceBindingAccountRepo{account: &Account{
		ID:          101,
		Name:        "mine",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		ProxyID:     &oldProxyID,
		Credentials: map[string]any{"api_key": "sk-test"},
	}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: &selfServiceBindingGroupRepo{}}

	updated, err := svc.UpdateAccount(selfServiceActorContext(), 101, &UpdateAccountInput{
		ProxyID:  &newProxyID,
		GroupIDs: &groupIDs,
	})

	require.NoError(t, err)
	require.Equal(t, &newProxyID, updated.ProxyID)
	require.Equal(t, groupIDs, repo.boundGroupIDs)
}
