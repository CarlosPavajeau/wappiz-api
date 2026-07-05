package middleware

import (
	"database/sql"
	"testing"
	"time"
	"wappiz/pkg/db"

	"github.com/stretchr/testify/require"
)

func TestIsBanned(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		user db.FindUserByIDRow
		want bool
	}{
		{
			name: "banned column is null",
			user: db.FindUserByIDRow{},
			want: false,
		},
		{
			name: "not banned",
			user: db.FindUserByIDRow{
				Banned: sql.NullBool{Bool: false, Valid: true},
			},
			want: false,
		},
		{
			name: "banned with no expiry is permanent",
			user: db.FindUserByIDRow{
				Banned: sql.NullBool{Bool: true, Valid: true},
			},
			want: true,
		},
		{
			name: "banned with future expiry",
			user: db.FindUserByIDRow{
				Banned:     sql.NullBool{Bool: true, Valid: true},
				BanExpires: sql.NullTime{Time: now.Add(time.Hour), Valid: true},
			},
			want: true,
		},
		{
			name: "banned with past expiry has lapsed",
			user: db.FindUserByIDRow{
				Banned:     sql.NullBool{Bool: true, Valid: true},
				BanExpires: sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isBanned(tt.user, now))
		})
	}
}
