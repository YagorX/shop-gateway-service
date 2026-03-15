package auth_grpc

import (
	"context"
	"fmt"

	auth_client "github.com/YagorX/shop-gateway/internal/client/grpc/auth"
)

type Repository struct {
	client *auth_client.Client
}

func NewRepository(client *auth_client.Client) (*Repository, error) {
	if client == nil {
		return nil, fmt.Errorf("auth grpc client is nil")
	}

	return &Repository{
		client: client,
	}, nil
}

func (r *Repository) Register(ctx context.Context, username, email, password string) (string, error) {
	resp, err := r.client.Register(ctx, username, email, password)
	if err != nil {
		return "", err
	}

	return resp.GetUserUuid(), nil
}

func (r *Repository) Login(
	ctx context.Context,
	emailOrName, password string,
	appID int64,
	deviceID string,
) (accessToken string, refreshToken string, err error) {
	resp, err := r.client.Login(ctx, emailOrName, password, appID, deviceID)
	if err != nil {
		return "", "", err
	}

	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (r *Repository) ValidateToken(ctx context.Context, token string, appID int64) (string, error) {
	resp, err := r.client.ValidateToken(ctx, token, appID)
	if err != nil {
		return "", err
	}

	return resp.GetUserUuid(), nil
}

func (r *Repository) Refresh(
	ctx context.Context,
	refreshToken string,
	appID int64,
	deviceID string,
) (accessToken string, newRefreshToken string, err error) {
	resp, err := r.client.Refresh(ctx, refreshToken, appID, deviceID)
	if err != nil {
		return "", "", err
	}

	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (r *Repository) Logout(ctx context.Context, refreshToken string, appID int64, deviceID string) error {
	_, err := r.client.Logout(ctx, refreshToken, appID, deviceID)
	return err
}

func (r *Repository) IsAdmin(ctx context.Context, userUUID string) (bool, error) {
	resp, err := r.client.IsAdmin(ctx, userUUID)
	if err != nil {
		return false, err
	}

	return resp.GetIsAdmin(), nil
}
