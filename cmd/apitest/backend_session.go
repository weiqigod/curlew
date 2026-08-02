package main

import (
	"context"

	"github.com/peterlindqvist/apitest/internal/backend"
	"github.com/peterlindqvist/apitest/internal/backend/device"
)

// loadStoredBackendAccessToken reads the short-lived backend API token from
// secure storage.
func loadStoredBackendAccessToken(configDir string) (string, error) {
	dev, err := device.Read(configDir)
	if err != nil {
		return "", err
	}
	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: configDir,
		DeviceID:  dev.DeviceID,
	})
	if err != nil {
		return "", err
	}
	return storage.GetAccessToken()
}

func hasStoredBackendRefreshToken(configDir string) bool {
	dev, err := device.Read(configDir)
	if err != nil {
		return false
	}
	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: configDir,
		DeviceID:  dev.DeviceID,
	})
	if err != nil {
		return false
	}
	_, err = storage.GetRefreshToken()
	return err == nil
}

// refreshStoredBackendAccessToken rotates the login session.
func refreshStoredBackendAccessToken(ctx context.Context, configDir, backendURL string) (string, error) {
	dev, err := device.Read(configDir)
	if err != nil {
		return "", err
	}
	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: configDir,
		DeviceID:  dev.DeviceID,
	})
	if err != nil {
		return "", err
	}
	client, err := backend.NewClient(backend.Options{BaseURL: backendURL})
	if err != nil {
		return "", err
	}
	tokens, err := backend.RefreshTokens(ctx, client, storage, configDir, dev.DeviceID)
	if err != nil {
		return "", err
	}
	if tokens.AccessToken != "" {
		return tokens.AccessToken, nil
	}
	return storage.GetAccessToken()
}
