package azure

import (
	"context"
	"time"
)

// Provider defines storage listing operations used by the TUI.
type Provider interface {
	ListAccounts(ctx context.Context) ([]Account, error)
	ListContainers(ctx context.Context, account string) ([]Container, error)
	ListBlobs(ctx context.Context, account, container string) ([]Blob, error)
}

type Account struct {
	Name   string
	Region string
}

type Container struct {
	Name         string
	PublicAccess string
}

type Blob struct {
	Name        string
	SizeBytes   int64
	Modified    time.Time
	ContentType string
}

// MockProvider is a placeholder data source for UI development.
type MockProvider struct {
	accounts   []Account
	containers map[string][]Container
	blobs      map[string]map[string][]Blob
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		accounts: []Account{
			{Name: "acme-dev", Region: "westeurope"},
			{Name: "acme-prod", Region: "eastus"},
		},
		containers: map[string][]Container{
			"acme-dev": {
				{Name: "images", PublicAccess: "private"},
				{Name: "logs", PublicAccess: "private"},
			},
			"acme-prod": {
				{Name: "backups", PublicAccess: "private"},
				{Name: "public", PublicAccess: "blob"},
			},
		},
		blobs: map[string]map[string][]Blob{
			"acme-dev": {
				"images": {
					{Name: "hero.jpg", SizeBytes: 312844, Modified: time.Date(2024, 5, 12, 10, 5, 0, 0, time.UTC), ContentType: "image/jpeg"},
					{Name: "logo.svg", SizeBytes: 4821, Modified: time.Date(2024, 4, 2, 14, 20, 0, 0, time.UTC), ContentType: "image/svg+xml"},
				},
				"logs": {
					{Name: "2024-05-10.log", SizeBytes: 982304, Modified: time.Date(2024, 5, 10, 3, 12, 0, 0, time.UTC), ContentType: "text/plain"},
					{Name: "2024-05-11.log", SizeBytes: 1048576, Modified: time.Date(2024, 5, 11, 3, 12, 0, 0, time.UTC), ContentType: "text/plain"},
				},
			},
			"acme-prod": {
				"backups": {
					{Name: "db-2024-05-01.bak", SizeBytes: 358717440, Modified: time.Date(2024, 5, 1, 1, 1, 0, 0, time.UTC), ContentType: "application/octet-stream"},
				},
				"public": {
					{Name: "robots.txt", SizeBytes: 58, Modified: time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC), ContentType: "text/plain"},
					{Name: "index.html", SizeBytes: 2214, Modified: time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC), ContentType: "text/html"},
				},
			},
		},
	}
}

func (m *MockProvider) ListAccounts(ctx context.Context) ([]Account, error) {
	_ = ctx
	return append([]Account(nil), m.accounts...), nil
}

func (m *MockProvider) ListContainers(ctx context.Context, account string) ([]Container, error) {
	_ = ctx
	containers := m.containers[account]
	return append([]Container(nil), containers...), nil
}

func (m *MockProvider) ListBlobs(ctx context.Context, account, container string) ([]Blob, error) {
	_ = ctx
	containers := m.blobs[account]
	blobs := containers[container]
	return append([]Blob(nil), blobs...), nil
}
