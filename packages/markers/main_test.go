package markers

import (
	"os"
	"testing"

	"github.com/iotaledger/goshimmer"
)

func TestMain(m *testing.M) {
	main.Setup()
	os.Exit(m.Run())
}
