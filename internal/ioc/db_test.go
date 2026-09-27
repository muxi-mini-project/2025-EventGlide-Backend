package ioc

import (
	"testing"
	"time"
)

func TestConnLifetimeOrDefaults(t *testing.T) {
	tests := []struct {
		name         string
		lifetime     time.Duration
		idle         time.Duration
		wantLifetime time.Duration
		wantIdle     time.Duration
	}{
		{"zero falls back to defaults", 0, 0, defaultConnMaxLifetime, defaultConnMaxIdleTime},
		{"negative falls back to defaults", -time.Second, -time.Minute, defaultConnMaxLifetime, defaultConnMaxIdleTime},
		{"sub-second misparse falls back to defaults", 3600 * time.Nanosecond, 500 * time.Millisecond, defaultConnMaxLifetime, defaultConnMaxIdleTime},
		{"only lifetime set defaults idle", 30 * time.Minute, 0, 30 * time.Minute, defaultConnMaxIdleTime},
		{"only idle set defaults lifetime", 0, 5 * time.Minute, defaultConnMaxLifetime, 5 * time.Minute},
		{"explicit values pass through", 30 * time.Minute, 5 * time.Minute, 30 * time.Minute, 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLifetime, gotIdle := connLifetimeOrDefaults(tt.lifetime, tt.idle)
			if gotLifetime != tt.wantLifetime || gotIdle != tt.wantIdle {
				t.Fatalf("connLifetimeOrDefaults(%v, %v) = (%v, %v), want (%v, %v)",
					tt.lifetime, tt.idle, gotLifetime, gotIdle, tt.wantLifetime, tt.wantIdle)
			}
		})
	}
}
