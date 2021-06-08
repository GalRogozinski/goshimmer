package main

import (
	"os"
	"testing"

	. "github.com/iotaledger/goshimmer/plugins/database"
	"github.com/iotaledger/hive.go/objectstorage"
	flag "github.com/spf13/pflag"
)

// TestMain should be called before all tests
func Setup() {
	overrideCache := flag.Int(CfgDatabaseOverrideCache, -1, "Globally overrides the number of seconds we cache data."+
		"If the value is less than 0 we don't override original configs") // call the tests
	objectstorage.SetCacheOverride(*overrideCache)
}
