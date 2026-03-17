package config

import (
	"fmt"
	"testing"
)

func TestInitConfig(t *testing.T) {
	cfg := InitConf()
	if cfg == nil {
		t.Fatal("Failed to init Config")
	}

	fmt.Printf("InitConfig: %+v\n", cfg)
}
